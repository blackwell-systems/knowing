package cppextractor

import (
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// --- Template class extraction ---

func TestExtractTemplateClass(t *testing.T) {
	src := `template<typename T>
class Vec {
    void push_back(T value);
};`
	nodes, _ := extract(t, "include/vec.hpp", src)

	// Should produce a type node for Vec.
	vec := findNode(nodes, types.KindType, "Vec")
	if vec == nil {
		t.Fatal("expected type node for Vec")
	}

	// Signature should contain template parameters.
	if !strings.Contains(vec.Signature, "template<typename T>") {
		t.Errorf("signature = %q, want to contain 'template<typename T>'", vec.Signature)
	}
	if !strings.Contains(vec.Signature, "class Vec") {
		t.Errorf("signature = %q, want to contain 'class Vec'", vec.Signature)
	}

	// QN should NOT contain template args (type erasure).
	if strings.Contains(vec.QualifiedName, "<") {
		t.Errorf("QN = %q, should not contain template args", vec.QualifiedName)
	}

	// Should have a push_back method.
	method := findNode(nodes, types.KindMethod, "push_back")
	if method == nil {
		t.Fatal("expected method node for push_back")
	}
}

func TestExtractTemplateStruct(t *testing.T) {
	src := `template<typename T, int N>
struct FixedArray {};`
	nodes, _ := extract(t, "include/fixed.hpp", src)

	arr := findNode(nodes, types.KindType, "FixedArray")
	if arr == nil {
		t.Fatal("expected type node for FixedArray")
	}

	if !strings.Contains(arr.Signature, "template<typename T, int N>") {
		t.Errorf("signature = %q, want template params", arr.Signature)
	}
	if !strings.Contains(arr.Signature, "struct FixedArray") {
		t.Errorf("signature = %q, want 'struct FixedArray'", arr.Signature)
	}
}

func TestExtractTemplateFunction(t *testing.T) {
	src := `template<typename T>
void sort(T* begin, T* end) {}`
	nodes, _ := extract(t, "src/algo.cpp", src)

	fn := findNode(nodes, types.KindFunction, "sort")
	if fn == nil {
		t.Fatal("expected function node for sort")
	}

	if !strings.Contains(fn.Signature, "template<typename T>") {
		t.Errorf("signature = %q, want template params", fn.Signature)
	}
	if !strings.Contains(fn.Signature, "void sort") {
		t.Errorf("signature = %q, want 'void sort'", fn.Signature)
	}
}

// --- Template specialization ---

func TestExtractFullSpecialization(t *testing.T) {
	src := `template<typename T>
class Vec {};

template<>
class Vec<bool> {};`
	nodes, edges := extract(t, "include/vec.hpp", src)

	// Primary template.
	primary := findNode(nodes, types.KindType, "Vec")
	if primary == nil {
		t.Fatal("expected type node for Vec")
	}

	// Specialization produces no additional node (same QN).
	// But should produce a specializes edge.
	hasSpecializesEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Specializes && e.TargetHash == primary.NodeHash {
			hasSpecializesEdge = true
			break
		}
	}
	if !hasSpecializesEdge {
		t.Error("expected specializes edge from specialization to primary Vec")
	}

	// Primary signature should be the template version.
	if !strings.Contains(primary.Signature, "template<typename T>") {
		t.Errorf("primary signature = %q, want template params", primary.Signature)
	}
}

func TestExtractPartialSpecialization(t *testing.T) {
	src := `template<typename T, typename U>
struct Pair {};

template<typename T>
struct Pair<T, T> {};`
	nodes, edges := extract(t, "include/pair.hpp", src)

	primary := findNode(nodes, types.KindType, "Pair")
	if primary == nil {
		t.Fatal("expected type node for Pair")
	}

	hasSpecializesEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Specializes && e.TargetHash == primary.NodeHash {
			hasSpecializesEdge = true
			break
		}
	}
	if !hasSpecializesEdge {
		t.Error("expected specializes edge from partial specialization to primary Pair")
	}

	// Primary should have the full param list.
	if !strings.Contains(primary.Signature, "typename T, typename U") {
		t.Errorf("primary signature = %q, want 'typename T, typename U'", primary.Signature)
	}
}

// --- Concept extraction ---

func TestExtractConcept(t *testing.T) {
	src := `template<typename T>
concept Sortable = requires(T a, T b) { a < b; };`
	nodes, _ := extract(t, "include/concepts.hpp", src)

	concept := findNode(nodes, types.KindType, "Sortable")
	if concept == nil {
		t.Fatal("expected type node for Sortable concept")
	}

	if !strings.Contains(concept.Signature, "concept Sortable") {
		t.Errorf("signature = %q, want 'concept Sortable'", concept.Signature)
	}
	if !strings.Contains(concept.Signature, "template<typename T>") {
		t.Errorf("signature = %q, want template params", concept.Signature)
	}
}

// --- Requires edges ---

func TestExtractRequiresEdge(t *testing.T) {
	src := `template<typename T>
concept Sortable = requires(T a, T b) { a < b; };

template<typename T> requires Sortable<T>
void filtered_sort(T* begin, T* end) {}`
	_, edges := extract(t, "include/algo.hpp", src)

	hasRequiresEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Requires {
			hasRequiresEdge = true
			break
		}
	}
	if !hasRequiresEdge {
		t.Error("expected requires edge from filtered_sort to Sortable concept")
	}
}

func TestExtractRequiresEdgeWithNamespace(t *testing.T) {
	src := `template<typename T> requires std::is_integral_v<T>
T add(T a, T b) { return a + b; }`
	_, edges := extract(t, "include/math.hpp", src)

	hasRequiresEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Requires {
			hasRequiresEdge = true
			break
		}
	}
	if !hasRequiresEdge {
		t.Error("expected requires edge from add to is_integral_v concept")
	}
}

// --- Instantiation edges ---

func TestExtractInstantiationEdge(t *testing.T) {
	src := `template<typename T>
class Vec {};

void caller() {
    Vec<int> v;
}`
	_, edges := extract(t, "src/main.cpp", src)

	hasInstantiatesEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Instantiates {
			hasInstantiatesEdge = true
			if e.Provenance != "ast_resolved" {
				t.Errorf("instantiates provenance = %q, want 'ast_resolved'", e.Provenance)
			}
			break
		}
	}
	if !hasInstantiatesEdge {
		t.Error("expected instantiates edge from caller to Vec template")
	}
}

func TestExtractExplicitInstantiation(t *testing.T) {
	src := `template<typename T>
class Vec {};

template class Vec<int>;`
	_, edges := extract(t, "src/main.cpp", src)

	hasInstantiatesEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Instantiates {
			hasInstantiatesEdge = true
			if e.Provenance != "ast_inferred" {
				t.Errorf("explicit instantiation provenance = %q, want 'ast_inferred'", e.Provenance)
			}
			break
		}
	}
	if !hasInstantiatesEdge {
		t.Error("expected instantiates edge from explicit instantiation to Vec")
	}
}

// --- Nested template in class ---

func TestExtractNestedTemplateClass(t *testing.T) {
	src := `class Outer {
    template<typename T>
    class Inner {};
};`
	nodes, _ := extract(t, "src/nested.cpp", src)

	outer := findNode(nodes, types.KindType, "Outer")
	if outer == nil {
		t.Fatal("expected type node for Outer")
	}

	inner := findNode(nodes, types.KindType, "Inner")
	if inner == nil {
		t.Fatal("expected type node for Inner")
	}

	// Inner's signature should contain template params.
	if !strings.Contains(inner.Signature, "template<typename T>") {
		t.Errorf("inner signature = %q, want template params", inner.Signature)
	}
}

// --- Template method in class ---

func TestExtractTemplateMethod(t *testing.T) {
	src := `class Widget {
    template<typename T>
    void render(T value);
};`
	nodes, _ := extract(t, "src/widget.cpp", src)

	render := findNode(nodes, types.KindMethod, "render")
	if render == nil {
		t.Fatal("expected method node for render")
	}

	if !strings.Contains(render.Signature, "template<typename T>") {
		t.Errorf("signature = %q, want template params", render.Signature)
	}
}

// --- Template alias ---

func TestExtractTemplateAlias(t *testing.T) {
	src := `template<typename T>
using VecAlias = std::vector<T>;`
	nodes, _ := extract(t, "include/aliases.hpp", src)

	alias := findNode(nodes, types.KindType, "VecAlias")
	if alias == nil {
		t.Fatal("expected type node for VecAlias")
	}

	if !strings.Contains(alias.Signature, "template<typename T>") {
		t.Errorf("signature = %q, want template params", alias.Signature)
	}
	if !strings.Contains(alias.Signature, "using VecAlias") {
		t.Errorf("signature = %q, want 'using VecAlias'", alias.Signature)
	}
}

// --- Template with inheritance ---

func TestExtractTemplateWithInheritance(t *testing.T) {
	src := `template<typename T>
class Base {};

template<typename T>
class Derived : public Base<T> {};`
	nodes, edges := extract(t, "src/inherit.cpp", src)

	derived := findNode(nodes, types.KindType, "Derived")
	if derived == nil {
		t.Fatal("expected type node for Derived")
	}

	if !strings.Contains(derived.Signature, "template<typename T>") {
		t.Errorf("signature = %q, want template params", derived.Signature)
	}

	// Should have extends edge.
	hasExtendsEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Extends && e.SourceHash == derived.NodeHash {
			hasExtendsEdge = true
			break
		}
	}
	if !hasExtendsEdge {
		t.Error("expected extends edge from Derived to Base")
	}
}

// --- Edge type constants ---

func TestSpecializesEdgeType(t *testing.T) {
	if edgetype.Specializes != "specializes" {
		t.Errorf("Specializes = %q, want 'specializes'", edgetype.Specializes)
	}
}

func TestInstantiatesEdgeType(t *testing.T) {
	if edgetype.Instantiates != "instantiates" {
		t.Errorf("Instantiates = %q, want 'instantiates'", edgetype.Instantiates)
	}
}

func TestRequiresEdgeType(t *testing.T) {
	if edgetype.Requires != "requires" {
		t.Errorf("Requires = %q, want 'requires'", edgetype.Requires)
	}
}

func TestSpecializesRWRWeight(t *testing.T) {
	w := RWRWeight(edgetype.Specializes)
	if w != 0.6 {
		t.Errorf("RWRWeight(Specializes) = %f, want 0.6", w)
	}
}

func TestInstantiatesRWRWeight(t *testing.T) {
	w := RWRWeight(edgetype.Instantiates)
	if w != 0.5 {
		t.Errorf("RWRWeight(Instantiates) = %f, want 0.5", w)
	}
}

func TestRequiresRWRWeight(t *testing.T) {
	w := RWRWeight(edgetype.Requires)
	if w != 0.4 {
		t.Errorf("RWRWeight(Requires) = %f, want 0.4", w)
	}
}

// --- Helper for RWR weight test (uses the function from edgetype package).

func RWRWeight(edgeType string) float64 {
	// Delegate to the actual implementation.
	// We import edgetype but need a wrapper for the test.
	return edgetype.RWRWeight(edgeType)
}

// --- Template header parsing ---

func TestExtractConceptName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Sortable<T>", "Sortable"},
		{"std::is_integral_v<T>", "is_integral_v"},
		{"std::same_as<T, int>", "same_as"},
		{"Sortable", "Sortable"},
	}
	for _, tt := range tests {
		got := extractConceptName(tt.input)
		if got != tt.want {
			t.Errorf("extractConceptName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- Complex scenarios ---

func TestExtractMultiParamTemplate(t *testing.T) {
	src := `template<typename T, typename U, int N>
class Triple {};`
	nodes, _ := extract(t, "include/triple.hpp", src)

	triple := findNode(nodes, types.KindType, "Triple")
	if triple == nil {
		t.Fatal("expected type node for Triple")
	}

	if !strings.Contains(triple.Signature, "typename T, typename U, int N") {
		t.Errorf("signature = %q, want multi-param template", triple.Signature)
	}
}

func TestExtractTemplateInNamespace(t *testing.T) {
	src := `namespace algo {
    template<typename T>
    void sort(T* begin, T* end) {}
}`
	nodes, edges := extract(t, "src/algo.cpp", src)

	ns := findNode(nodes, types.KindType, "algo")
	if ns == nil {
		t.Fatal("expected namespace node for algo")
	}

	sortFn := findNode(nodes, types.KindFunction, "sort")
	if sortFn == nil {
		t.Fatal("expected function node for sort")
	}

	if !strings.Contains(sortFn.Signature, "template<typename T>") {
		t.Errorf("signature = %q, want template params", sortFn.Signature)
	}

	// Containment edge from namespace to sort.
	hasContain := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Contains && e.SourceHash == ns.NodeHash && e.TargetHash == sortFn.NodeHash {
			hasContain = true
			break
		}
	}
	if !hasContain {
		t.Error("expected contains edge from algo namespace to sort function")
	}
}

func TestTemplateDeterministic(t *testing.T) {
	src := `template<typename T>
class Vec {};`

	nodes1, edges1 := extract(t, "include/vec.hpp", src)
	nodes2, edges2 := extract(t, "include/vec.hpp", src)

	if len(nodes1) != len(nodes2) {
		t.Fatalf("node count mismatch: %d vs %d", len(nodes1), len(nodes2))
	}
	for i := range nodes1 {
		if nodes1[i].QualifiedName != nodes2[i].QualifiedName {
			t.Errorf("node[%d] QN mismatch: %q vs %q", i, nodes1[i].QualifiedName, nodes2[i].QualifiedName)
		}
		if nodes1[i].Signature != nodes2[i].Signature {
			t.Errorf("node[%d] signature mismatch: %q vs %q", i, nodes1[i].Signature, nodes2[i].Signature)
		}
	}
	if len(edges1) != len(edges2) {
		t.Fatalf("edge count mismatch: %d vs %d", len(edges1), len(edges2))
	}
	for i := range edges1 {
		if edges1[i].EdgeHash != edges2[i].EdgeHash {
			t.Errorf("edge[%d] hash mismatch", i)
		}
	}
}
