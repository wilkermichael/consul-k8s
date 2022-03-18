package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&PeeringToken{}, &PeeringTokenList{})
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PeeringToken is the Schema for the peeringtokens API
type PeeringToken struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PeeringTokenSpec   `json:"spec,omitempty"`
	Status PeeringTokenStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PeeringTokenList contains a list of PeeringToken
type PeeringTokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PeeringToken `json:"items"`
}

// PeeringTokenSpec defines the desired state of PeeringToken
type PeeringTokenSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of PeeringToken. Edit peeringtoken_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// PeeringTokenStatus defines the observed state of PeeringToken
type PeeringTokenStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}
