package context

// containersEquivalenceClasses returns equivalence classes for containerization.
func containersEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "DOCKER_IMAGE",
		Phrases:    []string{"docker image", "container image", "dockerfile", "build image", "image layer"},
		Targets:    []string{"Dockerfile", "Image", "ImageBuild", "ImagePull", "ImagePush", "Layer"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "DOCKER_COMPOSE",
		Phrases:    []string{"docker compose", "compose file", "multi-container", "service definition"},
		Targets:    []string{"ComposeFile", "Service", "Network", "Volume", "Compose"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CONTAINER_RUNTIME",
		Phrases:    []string{"container runtime", "container lifecycle", "container start", "container stop", "container exec"},
		Targets:    []string{"Container", "ContainerRuntime", "RunContainer", "StartContainer", "StopContainer", "ExecInContainer"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CONTAINER_REGISTRY",
		Phrases:    []string{"container registry", "image registry", "push image", "pull image", "registry auth"},
		Targets:    []string{"Registry", "RegistryClient", "Push", "Pull", "Authenticate", "Repository"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
	}
}
