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

func (opts *ConfigMapOptions) applyDefaults() {
	if opts.Labels == nil {
		opts.Labels = make(map[string]string)
	}
	if opts.Data == nil {
		opts.Data = make(map[string]string)
	}
}

func (opts *NamespaceOptions) applyDefaults() {
	if opts.Labels == nil {
		opts.Labels = make(map[string]string)
	}
}

func (opts *NamespaceLabelOptions) applyDefaults() {
	if opts.Labels == nil {
		opts.Labels = make(map[string]string)
	}
	if opts.SpecLabels == nil {
		opts.SpecLabels = make(map[string]string)
	}
}
