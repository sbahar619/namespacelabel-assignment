package factory

type ConfigMapOptions struct {
	Name        string
	Namespace   string
	Data        map[string]string
	Labels      map[string]string
	Annotations map[string]string
}

type NamespaceOptions struct {
	Name        string
	Labels      map[string]string
	Annotations map[string]string
}

type NamespaceLabelOptions struct {
	Name        string
	Namespace   string
	Labels      map[string]string
	Annotations map[string]string
	Finalizers  []string
	SpecLabels  map[string]string
}
