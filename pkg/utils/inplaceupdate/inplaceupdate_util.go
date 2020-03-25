package inplaceupdate

import (
	"context"
	"encoding/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"strconv"
	"strings"

	"gomodules.xyz/jsonpatch/v2"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"

	kromev1 "krome/pkg/apis/apps/v1"
)

var (
	inPlaceUpdatePatchRexp = regexp.MustCompile("^/spec/containers/([0-9]+)/image$")
)

// UpdateSpec records the images of containers which need to in-place update.
type UpdateSpec struct {
	Revision        string
	ContainerImages map[string]string
	MetaDataPatch   []byte
}

// CalculateInPlaceUpdateSpec calculates diff between old and update revisions.
// If the diff just contains replace operation of spec.containers[x].image, it will returns an UpdateSpec.
// Otherwise, it returns nil which means can not use in-place update.
func CalculateInPlaceUpdateSpec(oldRevision, newRevision *appsv1.ControllerRevision) *UpdateSpec {
	if oldRevision == nil || newRevision == nil {
		return nil
	}

	patches, err := jsonpatch.CreatePatch(oldRevision.Data.Raw, newRevision.Data.Raw)
	if err != nil {
		return nil
	}

	oldTemp, err := getTemplateFromRevision(oldRevision)
	if err != nil {
		return nil
	}
	newTemp, err := getTemplateFromRevision(newRevision)
	if err != nil {
		return nil
	}

	updateSpec := &UpdateSpec{
		Revision:        newRevision.Name,
		ContainerImages: make(map[string]string),
	}

	// all patches for podSpec can just update images
	var metadataChanged bool
	for _, jsonPatchOperation := range patches {
		jsonPatchOperation.Path = strings.Replace(jsonPatchOperation.Path, "/spec/template", "", 1)

		if !strings.HasPrefix(jsonPatchOperation.Path, "/spec/") {
			metadataChanged = true
			continue
		}
		if jsonPatchOperation.Operation != "replace" || !inPlaceUpdatePatchRexp.MatchString(jsonPatchOperation.Path) {
			return nil
		}
		// for example: /spec/containers/0/image
		words := strings.Split(jsonPatchOperation.Path, "/")
		idx, _ := strconv.Atoi(words[3])
		if len(oldTemp.Spec.Containers) <= idx {
			return nil
		}
		updateSpec.ContainerImages[oldTemp.Spec.Containers[idx].Name] = jsonPatchOperation.Value.(string)
	}
	if metadataChanged {
		oldBytes, _ := json.Marshal(v1.Pod{ObjectMeta: oldTemp.ObjectMeta})
		newBytes, _ := json.Marshal(v1.Pod{ObjectMeta: newTemp.ObjectMeta})
		patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldBytes, newBytes, &v1.Pod{})
		if err != nil {
			return nil
		}
		updateSpec.MetaDataPatch = patchBytes
	}
	return updateSpec
}

func UpdateCondition(mgr manager.Manager, pod *v1.Pod) error {
	newCondition := v1.PodCondition{
		Type:               kromev1.InPlaceUpdateReady,
		LastTransitionTime: metav1.Now(),
		Status:             v1.ConditionFalse,
		Reason:             "Starting inplace update",
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var p = &v1.Pod{}
		if err := mgr.GetClient().Get(context.TODO(), types.NamespacedName{pod.Namespace, pod.Name}, p); err != nil {
			return err
		}

		for i, c := range pod.Status.Conditions {
			if c.Type == newCondition.Type {
				if c.Status != newCondition.Status {
					pod.Status.Conditions[i] = newCondition
				}
				return nil
			}
		}
		pod.Status.Conditions = append(pod.Status.Conditions, newCondition)
		return nil
	})
}

func UpdatePodInPlate(mgr manager.Manager, namespace, name string, spec *UpdateSpec) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var po = &v1.Pod{}
		if err := mgr.GetClient().Get(context.TODO(), types.NamespacedName{namespace, name}, po); err != nil {
			return err
		}

		for i := range po.Spec.Containers {
			if newImage, ok := spec.ContainerImages[po.Spec.Containers[i].Name]; ok {
				po.Spec.Containers[i].Image = newImage
			}
		}
		po.Labels[appsv1.ControllerRevisionHashLabelKey] = spec.Revision

		// TODO record

		return mgr.GetClient().Update(context.TODO(), po, &client.UpdateOptions{})
	})
}

func getTemplateFromRevision(revision *appsv1.ControllerRevision) (*v1.PodTemplateSpec, error) {
	var patchObj *struct {
		Spec struct {
			Template v1.PodTemplateSpec `json:"template"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(revision.Data.Raw, &patchObj); err != nil {
		return nil, err
	}
	return &patchObj.Spec.Template, nil
}
