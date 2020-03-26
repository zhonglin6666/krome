package inplaceupdate

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"

	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	hashutil "k8s.io/kubernetes/pkg/util/hash"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// byRevision implements sort.Interface to allow ControllerRevisions to be sorted by Revision.
type byRevision []*apps.ControllerRevision

func (br byRevision) Len() int {
	return len(br)
}

// Less breaks ties first by creation timestamp, then by name
func (br byRevision) Less(i, j int) bool {
	if br[i].Revision == br[j].Revision {
		if br[j].CreationTimestamp.Equal(&br[i].CreationTimestamp) {
			return br[i].Name < br[j].Name
		}
		return br[j].CreationTimestamp.After(br[i].CreationTimestamp.Time)
	}
	return br[i].Revision < br[j].Revision
}

func (br byRevision) Swap(i, j int) {
	br[i], br[j] = br[j], br[i]
}

// SortControllerRevisions sorts revisions by their Revision.
func SortControllerRevisions(revisions []*apps.ControllerRevision) {
	sort.Stable(byRevision(revisions))
}

func AdoptControllerRevision(client clientset.Interface, parent metav1.Object, parentKind schema.GroupVersionKind, revision *apps.ControllerRevision) (*apps.ControllerRevision, error) {
	// Return an error if the parent does not own the revision
	if owner := metav1.GetControllerOf(revision); owner != nil {
		return nil, fmt.Errorf("attempt to adopt revision owned by %v", owner)
	}

	// Use strategic merge patch to add an owner reference indicating a controller ref
	return client.AppsV1().ControllerRevisions(parent.GetNamespace()).Patch(revision.GetName(),
		types.StrategicMergePatchType, []byte(fmt.Sprintf(
			`{"metadata":{"ownerReferences":[{"apiVersion":"%s","kind":"%s","name":"%s","uid":"%s","controller":true,"blockOwnerDeletion":true}],"uid":"%s"}}`,
			parentKind.GroupVersion().String(), parentKind.Kind,
			parent.GetName(), parent.GetUID(), revision.UID)))
}

func CreateControllerRevision(mgr manager.Manager, c clientset.Interface, parent metav1.Object, revision *apps.ControllerRevision, collisionCount *int32) (*apps.ControllerRevision, error) {
	if collisionCount == nil {
		return nil, fmt.Errorf("collisionCount should not be nil")
	}

	// Clone the input
	clone := revision.DeepCopy()

	// Continue to attempt to create the revision updating the name with a new hash on each iteration
	for {
		hash := HashControllerRevision(revision, collisionCount)
		// Update the revisions name
		clone.Name = ControllerRevisionName(parent.GetName(), hash)
		ns := parent.GetNamespace()
		err := mgr.GetClient().Create(context.TODO(), clone, &client.CreateOptions{})
		if errors.IsAlreadyExists(err) {
			exists, err := c.AppsV1().ControllerRevisions(ns).Get(clone.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			if bytes.Equal(exists.Data.Raw, clone.Data.Raw) {
				return exists, nil
			}
			*collisionCount++
			continue
		}
		return clone, err
	}
}

func UpdateControllerRevision(mgr manager.Manager, revision *apps.ControllerRevision, newRevision int64) (*apps.ControllerRevision, error) {
	clone := revision.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if clone.Revision == newRevision {
			return nil
		}
		clone.Revision = newRevision

		updateErr := mgr.GetClient().Update(context.TODO(), clone, &client.UpdateOptions{})
		if updateErr == nil {
			return nil
		}

		var cr = &apps.ControllerRevision{}
		if err := mgr.GetClient().Get(context.TODO(), types.NamespacedName{clone.Namespace, clone.Name}, cr); err != nil {
			// make a copy so we don't mutate the shared cache
			clone = cr.DeepCopy()
		}
		return updateErr
	})
	return clone, err
}

// HashControllerRevision hashes the contents of revision's Data using FNV hashing. If probe is not nil, the byte value
// of probe is added written to the hash as well. The returned hash will be a safe encoded string to avoid bad words.
func HashControllerRevision(revision *apps.ControllerRevision, probe *int32) string {
	hf := fnv.New32()
	if len(revision.Data.Raw) > 0 {
		hf.Write(revision.Data.Raw)
	}
	if revision.Data.Object != nil {
		hashutil.DeepHashObject(hf, revision.Data.Object)
	}
	if probe != nil {
		hf.Write([]byte(strconv.FormatInt(int64(*probe), 10)))
	}
	return rand.SafeEncodeString(fmt.Sprint(hf.Sum32()))
}

// ControllerRevisionName returns the Name for a ControllerRevision in the form prefix-hash. If the length
// of prefix is greater than 223 bytes, it is truncated to allow for a name that is no larger than 253 bytes.
func ControllerRevisionName(prefix string, hash string) string {
	if len(prefix) > 223 {
		prefix = prefix[:223]
	}

	return fmt.Sprintf("%s-%s", prefix, hash)
}
