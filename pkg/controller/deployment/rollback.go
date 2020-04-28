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
	"fmt"
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
			return r.updateDeploymentAndClearRollbackTo(d)
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
			// rollback by copying podTemplate.Spec from the replica set
			// revision number will be incremented during the next getAllReplicaSetsAndSyncRevision call
			// no-op if the spec matches current deployment's podTemplate.Spec
			performedRollback, err := r.rollbackToTemplate(d, rs)
			if performedRollback && err == nil {
				r.emitRollbackNormalEvent(d, fmt.Sprintf("Rolled back deployment %q to revision %d", d.Name, rollbackTo.Revision))
			}
			return err
		}
	}
	r.emitRollbackWarningEvent(d, deploymentutil.RollbackRevisionNotFound, "Unable to find the revision to rollback to.")
	// Gives up rollback
	return r.updateDeploymentAndClearRollbackTo(d)
}

// rollbackToTemplate compares the templates of the provided deployment and replica set and
// updates the deployment with the replica set template in case they are different. It also
// cleans up the rollback spec so subsequent requeues of the deployment won't end up in here.
func (r *ReconcileDeployment) rollbackToTemplate(d *kromev1.Deployment, rs *kromev1.ReplicaSet) (bool, error) {
	performedRollback := false
	if !deploymentutil.EqualIgnoreHash(&d.Spec.Template, &rs.Spec.Template) {
		logrus.Infof("Rolling back deployment %q to template spec %+v", d.Name, rs.Spec.Template.Spec)
		setFromReplicaSetTemplate(d, rs.Spec.Template)
		// set RS (the old RS we'll rolling back to) annotations back to the deployment;
		// otherwise, the deployment's current annotations (should be the same as current new RS) will be copied to the RS after the rollback.
		//
		// For example,
		// A Deployment has old RS1 with annotation {change-cause:create}, and new RS2 {change-cause:edit}.
		// Note that both annotations are copied from Deployment, and the Deployment should be annotated {change-cause:edit} as well.
		// Now, rollback Deployment to RS1, we should update Deployment's pod-template and also copy annotation from RS1.
		// Deployment is now annotated {change-cause:create}, and we have new RS1 {change-cause:create}, old RS2 {change-cause:edit}.
		//
		// If we don't copy the annotations back from RS to deployment on rollback, the Deployment will stay as {change-cause:edit},
		// and new RS1 becomes {change-cause:edit} (copied from deployment after rollback), old RS2 {change-cause:edit}, which is not correct.
		setDeploymentAnnotationsTo(d, rs)
		performedRollback = true
	} else {
		logrus.Infof("Rolling back to a revision that contains the same template as current deployment %q, skipping rollback...", d.Name)
		eventMsg := fmt.Sprintf("The rollback revision contains the same template as current deployment %q", d.Name)
		r.emitRollbackWarningEvent(d, deploymentutil.RollbackTemplateUnchanged, eventMsg)
	}

	return performedRollback, r.updateDeploymentAndClearRollbackTo(d)
}

// updateDeploymentAndClearRollbackTo sets .spec.rollbackTo to nil and update the input deployment
// It is assumed that the caller will have updated the deployment template appropriately (in case
// we want to rollback).
func (r *ReconcileDeployment) updateDeploymentAndClearRollbackTo(d *kromev1.Deployment) error {
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
