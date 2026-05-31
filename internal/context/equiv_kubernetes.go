package context

// kubernetesEquivalenceClasses returns equivalence classes for kubernetes.
func kubernetesEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "K8S_CONTROLLER",
		Phrases:    []string{"controller", "reconciler", "reconciliation", "controller manager", "watch resource"},
		Targets:    []string{"Controller", "Reconciler", "Reconcile", "ControllerManager", "SharedInformerFactory", "NewController"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "go",
		},
		{
		Concept:    "K8S_ADMISSION",
		Phrases:    []string{"admission webhook", "admission controller", "mutating webhook", "validating webhook", "admission plugin"},
		Targets:    []string{"AdmissionHandler", "MutatingWebhook", "ValidatingWebhook", "Admit", "Validate", "WebhookServer", "AdmissionReview"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "go",
		},
		{
		Concept:    "K8S_SCHEDULER",
		Phrases:    []string{"scheduler", "scheduling", "pod scheduling", "scheduling queue", "predicate", "priority", "node filter"},
		Targets:    []string{"Scheduler", "SchedulingQueue", "Filter", "Score", "Reserve", "Permit", "PreFilter", "PostFilter", "NodeInfo", "Framework"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "go",
		},
		{
		Concept:    "K8S_INFORMER",
		Phrases:    []string{"informer", "watch", "list watch", "cache", "shared informer", "event handler"},
		Targets:    []string{"SharedInformerFactory", "Informer", "Lister", "NewInformer", "AddEventHandler", "ResourceEventHandlerFuncs", "WatchFunc"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "go",
		},
		{
		Concept:    "K8S_API_SERVER",
		Phrases:    []string{"api server", "etcd", "resource version", "watch event", "api request"},
		Targets:    []string{"GenericAPIServer", "Storage", "RESTStorage", "Watcher", "WatchServer", "ResourceVersion"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "go",
		},
		{
		Concept:    "K8S_API",
		Phrases:    []string{"api server", "admission", "webhook", "api resource", "api group"},
		Targets:    []string{"Admit", "Validate", "Mutate", "AdmissionReview", "AdmissionResponse", "WebhookHandler", "Register", "AddToScheme", "GroupVersion"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "K8S_WORKLOAD",
		Phrases:    []string{"pod", "deployment", "statefulset", "daemonset", "job", "container"},
		Targets:    []string{"Pod", "PodSpec", "Container", "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
	}
}
