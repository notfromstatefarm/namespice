package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type NamespaceClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Resources         []map[string]interface{} `json:"resources"`
}

type NamespaceClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []NamespaceClass `json:"items"`
}
