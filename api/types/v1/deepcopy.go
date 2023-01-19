package v1

import (
	"encoding/json"
	"k8s.io/apimachinery/pkg/runtime"
)

func (in *NamespaceClass) DeepCopyInto(out *NamespaceClass) {
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = in.ObjectMeta
	out.Resources = make([]map[string]interface{}, 0)

	for _, r := range in.Resources {
		jsonStr, err := json.Marshal(r)
		if err == nil {
			dest := make(map[string]interface{})
			err = json.Unmarshal(jsonStr, &dest)
			if err == nil {
				out.Resources = append(out.Resources, dest)
			}
		}
	}
}

func (in *NamespaceClass) DeepCopyObject() runtime.Object {
	out := NamespaceClass{}
	in.DeepCopyInto(&out)

	return &out
}

func (in *NamespaceClassList) DeepCopyObject() runtime.Object {
	out := NamespaceClassList{}
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta

	if in.Items != nil {
		out.Items = make([]NamespaceClass, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}

	return &out
}
