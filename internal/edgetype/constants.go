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

	// Structural edges (derived from QN hierarchy)
	Contains  = "contains"   // type/class -> method/field
	MemberOf  = "member_of"  // method/field -> type/class (reverse of contains)

	// P2/P3 static edge types
	Documents        = "documents"
	ConsumesEndpoint = "consumes_endpoint"
	ImplementsRPC    = "implements_rpc"
	ConsumesRPC      = "consumes_rpc"
	GatedByFlag      = "gated_by_flag"
	DeployedBy       = "deployed_by"
	TestedBy         = "tested_by"
)

// RWRWeight returns the Random Walk with Restart weight for the given edge type.
// Unknown edge types return 0.3 (the default).
func RWRWeight(edgeType string) float64 {
	switch edgeType {
	case Calls:
		return 1.0
	case Implements:
		return 0.8
	case Overrides:
		return 0.8
	case HandlesRoute:
		return 0.7
	case Extends:
		return 0.7
	case Contains:
		return 0.8
	case MemberOf:
		return 0.6
	case Imports:
		return 0.5
	case DependsOn:
		return 0.5
	case Tests:
		return 0.6
	case References:
		return 0.4
	case Throws:
		return 0.4
	case Decorates:
		return 0.3
	case OwnedBy, AuthoredBy:
		return 0.0
	case Documents:
		return 0.2
	case ConsumesEndpoint:
		return 0.5
	case ImplementsRPC:
		return 0.8
	case ConsumesRPC:
		return 0.6
	case GatedByFlag:
		return 0.3
	case DeployedBy:
		return 0.4
	case TestedBy:
		return 0.5
	default:
		return 0.3
	}
}
