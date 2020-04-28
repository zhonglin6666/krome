/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deployment

import (
	"context"
	"strconv"

	"github.com/sirupsen/logrus"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	deploymentutil "k8s.io/kubernetes/pkg/controller/deployment/util"

	kromev1 "krome.io/krome/pkg/apis/apps/v1"
)

func (r *ReconcileDeployment) rollback(d *kromev1.Deployment, rsList []*kromev1.ReplicaSet) error {
	newRS, allOldRSs, err := r.getAllReplicaSetsAndSyncRevision(d, rsList, true)
	if err != nil {
		return err
	}

	allRSs := append(allOldRSs, newRS)
	rollbackTo := getRollbackTo(d)
	// If rollback revision is 0, rollback to the last revision
	if rollbackTo.Revision == 0 {
		if rollbackTo.Revision = lastRevision(allRSs); rollbackTo.Revision == 0 {
			// If we still can't find the last revision, gives up rollback
			r.emitRollbackWarningEvent(d, deploymentutil.RollbackRevisionNotFound, "Unable to find last revision")
			// Gives up rollback
			return r.updateDeploymentAndClearRollback(d)
		}
	}

	for _, rs := range allRSs {
		v, err := revision(rs)
		if err != nil {
			logrus.Warningf("Unable to extract revision form deployment's replica set: %s error: %v", rs.Name, err)
			continue
		}
		if v == rollbackTo.Revision {
			logrus.Infof("Found replica set %s with desired revision: %d", rs.Name, v)

		}
	}
}

// updateDeploymentAndClearRollbackTo sets .spec.rollbackTo to nil and update the input deployment
// It is assumed that the caller will have updated the deployment template appropriately (in case
// we want to rollback).
func (r *ReconcileDeployment) updateDeploymentAndClearRollback(d *kromev1.Deployment) error {
	logrus.Infof("Cleans up rollbackTo of deployment %s", d.Name)
	setRollbackTo(d, nil)
	_, err := r.kromeClient.AppsV1().Deployments(d.Namespace).Update(context.TODO(), d, metav1.UpdateOptions{})
	return err
}

func (r *ReconcileDeployment) emitRollbackWarningEvent(d *kromev1.Deployment, reason, message string) {
	r.recorder.Eventf(d, v1.EventTypeWarning, reason, message)
}

func (r *ReconcileDeployment) emitRollbackNormalEvent(d *kromev1.Deployment, message string) {
	r.recorder.Eventf(d, v1.EventTypeNormal, deploymentutil.RollbackDone, message)
}

// TODO: Remove this when extensions/v1beta1 and apps/v1beta1 Deployment are dropped.
func getRollbackTo(d *kromev1.Deployment) *extensions.RollbackConfig {
	// Extract the annotation used for round-tripping the deprecated RollbackTo field.
	revision := d.Annotations[apps.DeprecatedRollbackTo]
	if revision == "" {
		return nil
	}

	revision64, err := strconv.ParseInt(revision, 10, 64)
	if err != nil {
		return nil
	}
	return &extensions.RollbackConfig{
		Revision: revision64,
	}
}

// TODO: Remove this when extensions/v1beta1 and apps/v1beat1 Deployment are dropped.
func setRollbackTo(d *kromev1.Deployment, rollbackTo *extensions.RollbackConfig) {
	if d.Annotations == nil {
		d.Annotations = make(map[string]string)
	}

	if rollbackTo == nil {
		delete(d.Annotations, apps.DeprecatedRollbackTo)
		return
	}

	d.Annotations[apps.DeprecatedRollbackTo] = strconv.FormatInt(rollbackTo.Revision, 10)
}
