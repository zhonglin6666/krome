// +build !ignore_autogenerated

// Code generated by operator-sdk. DO NOT EDIT.

package v1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RollingUpdateStatefulSetStrategy) DeepCopyInto(out *RollingUpdateStatefulSetStrategy) {
	*out = *in
	if in.Partition != nil {
		in, out := &in.Partition, &out.Partition
		*out = new(int32)
		**out = **in
	}
	if in.MaxUnavailable != nil {
		in, out := &in.MaxUnavailable, &out.MaxUnavailable
		*out = new(intstr.IntOrString)
		**out = **in
	}
	if in.UnorderedUpdate != nil {
		in, out := &in.UnorderedUpdate, &out.UnorderedUpdate
		*out = new(UnorderedUpdateStrategy)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RollingUpdateStatefulSetStrategy.
func (in *RollingUpdateStatefulSetStrategy) DeepCopy() *RollingUpdateStatefulSetStrategy {
	if in == nil {
		return nil
	}
	out := new(RollingUpdateStatefulSetStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StatefulSetUpdateStrategy) DeepCopyInto(out *StatefulSetUpdateStrategy) {
	*out = *in
	if in.RollingUpdate != nil {
		in, out := &in.RollingUpdate, &out.RollingUpdate
		*out = new(RollingUpdateStatefulSetStrategy)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StatefulSetUpdateStrategy.
func (in *StatefulSetUpdateStrategy) DeepCopy() *StatefulSetUpdateStrategy {
	if in == nil {
		return nil
	}
	out := new(StatefulSetUpdateStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Statefulset) DeepCopyInto(out *Statefulset) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Statefulset.
func (in *Statefulset) DeepCopy() *Statefulset {
	if in == nil {
		return nil
	}
	out := new(Statefulset)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Statefulset) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StatefulsetList) DeepCopyInto(out *StatefulsetList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Statefulset, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StatefulsetList.
func (in *StatefulsetList) DeepCopy() *StatefulsetList {
	if in == nil {
		return nil
	}
	out := new(StatefulsetList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *StatefulsetList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StatefulsetSpec) DeepCopyInto(out *StatefulsetSpec) {
	*out = *in
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	if in.Selector != nil {
		in, out := &in.Selector, &out.Selector
		*out = new(metav1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	in.Template.DeepCopyInto(&out.Template)
	if in.VolumeClaimTemplates != nil {
		in, out := &in.VolumeClaimTemplates, &out.VolumeClaimTemplates
		*out = make([]corev1.PersistentVolumeClaim, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.UpdateStrategy.DeepCopyInto(&out.UpdateStrategy)
	if in.RevisionHistoryLimit != nil {
		in, out := &in.RevisionHistoryLimit, &out.RevisionHistoryLimit
		*out = new(int32)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StatefulsetSpec.
func (in *StatefulsetSpec) DeepCopy() *StatefulsetSpec {
	if in == nil {
		return nil
	}
	out := new(StatefulsetSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StatefulsetStatus) DeepCopyInto(out *StatefulsetStatus) {
	*out = *in
	if in.ObservedGeneration != nil {
		in, out := &in.ObservedGeneration, &out.ObservedGeneration
		*out = new(int64)
		**out = **in
	}
	if in.CollisionCount != nil {
		in, out := &in.CollisionCount, &out.CollisionCount
		*out = new(int32)
		**out = **in
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]appsv1.StatefulSetCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StatefulsetStatus.
func (in *StatefulsetStatus) DeepCopy() *StatefulsetStatus {
	if in == nil {
		return nil
	}
	out := new(StatefulsetStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UnorderedUpdateStrategy) DeepCopyInto(out *UnorderedUpdateStrategy) {
	*out = *in
	if in.PriorityStrategy != nil {
		in, out := &in.PriorityStrategy, &out.PriorityStrategy
		*out = new(UpdatePriorityStrategy)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UnorderedUpdateStrategy.
func (in *UnorderedUpdateStrategy) DeepCopy() *UnorderedUpdateStrategy {
	if in == nil {
		return nil
	}
	out := new(UnorderedUpdateStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpdatePriorityOrderTerm) DeepCopyInto(out *UpdatePriorityOrderTerm) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpdatePriorityOrderTerm.
func (in *UpdatePriorityOrderTerm) DeepCopy() *UpdatePriorityOrderTerm {
	if in == nil {
		return nil
	}
	out := new(UpdatePriorityOrderTerm)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpdatePriorityStrategy) DeepCopyInto(out *UpdatePriorityStrategy) {
	*out = *in
	if in.OrderPriority != nil {
		in, out := &in.OrderPriority, &out.OrderPriority
		*out = make([]UpdatePriorityOrderTerm, len(*in))
		copy(*out, *in)
	}
	if in.WeightPriority != nil {
		in, out := &in.WeightPriority, &out.WeightPriority
		*out = make([]UpdatePriorityWeightTerm, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpdatePriorityStrategy.
func (in *UpdatePriorityStrategy) DeepCopy() *UpdatePriorityStrategy {
	if in == nil {
		return nil
	}
	out := new(UpdatePriorityStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpdatePriorityWeightTerm) DeepCopyInto(out *UpdatePriorityWeightTerm) {
	*out = *in
	in.MatchSelector.DeepCopyInto(&out.MatchSelector)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpdatePriorityWeightTerm.
func (in *UpdatePriorityWeightTerm) DeepCopy() *UpdatePriorityWeightTerm {
	if in == nil {
		return nil
	}
	out := new(UpdatePriorityWeightTerm)
	in.DeepCopyInto(out)
	return out
}
