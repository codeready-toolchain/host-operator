package constants

import toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

// HostOperatorFieldManager is the field manager we want to use in the managed fields
// of objects deployed by the host operator.
const HostOperatorFieldManager = "kubesaw-host-operator"

// BundledWithHostOperatorAnnotationValue is meant to be the value of the toolchainv1alpha1.BundledLabelKey that marks
// the objects as bundled with the host operator and therefore managed by it.
const BundledWithHostOperatorAnnotationValue = "host-operator"

const BundledNSTemplateTierFinalizerName = toolchainv1alpha1.LabelKeyPrefix + "bundled-tier"
