package edgetype

// Edge type constants for the knowing knowledge graph.
// These constants define the canonical string identifiers for each edge type.
const (
	Calls           = "calls"
	Imports         = "imports"
	Implements      = "implements"
	References      = "references"
	HandlesRoute    = "handles_route"
	DependsOn       = "depends_on"
	Deploys         = "deploys"
	Exposes         = "exposes"
	Configures      = "configures"
	Publishes       = "publishes"
	Subscribes      = "subscribes"
	ConnectsTo      = "connects_to"
	Extends         = "extends"
	Overrides       = "overrides"
	Decorates       = "decorates"
	Throws          = "throws"
	OwnedBy         = "owned_by"
	AuthoredBy      = "authored_by"
	Tests           = "tests"
	RuntimeCalls    = "runtime_calls"
	RuntimeRPC      = "runtime_rpc"
	RuntimeProduces = "runtime_produces"
	RuntimeConsumes = "runtime_consumes"
)
