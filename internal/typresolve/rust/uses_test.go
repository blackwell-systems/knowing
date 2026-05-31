package rustresolve

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/rust"
)

func parseRust(t *testing.T, src string) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(rust.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	t.Cleanup(func() { tree.Close() })
	return tree.RootNode()
}

func TestResolveUsePath_Crate(t *testing.T) {
	modulePath, isExternal := ResolveUsePath("crate::core::resolver::FeatureResolver", "src/main.rs")
	if modulePath != "crate::core::resolver" {
		t.Errorf("modulePath = %q, want %q", modulePath, "crate::core::resolver")
	}
	if isExternal {
		t.Error("isExternal = true, want false")
	}
}

func TestResolveUsePath_Self(t *testing.T) {
	modulePath, isExternal := ResolveUsePath("self::helpers::resolve", "src/core/mod.rs")
	if isExternal {
		t.Error("isExternal = true, want false")
	}
	// self from src/core/mod.rs -> crate::core, then self::helpers -> crate::core::helpers
	if modulePath != "crate::core::helpers" {
		t.Errorf("modulePath = %q, want %q", modulePath, "crate::core::helpers")
	}
}

func TestResolveUsePath_Super(t *testing.T) {
	modulePath, isExternal := ResolveUsePath("super::utils", "src/core/resolver.rs")
	if isExternal {
		t.Error("isExternal = true, want false")
	}
	// super from src/core/resolver.rs -> parent is crate::core, then super -> crate
	// super::utils -> crate::utils... wait: current module is crate::core::resolver,
	// super prefix is just "super", ResolveModulePath("super", file) -> parent = crate::core
	// Then the full path is super::utils, so prefix is "super" (1 segment), resolved = crate::core
	// Actually ResolveUsePath splits on :: -> ["super", "utils"], prefix = segments[:1] = ["super"]
	// joined = "super", ResolveModulePath("super", "src/core/resolver.rs"):
	//   segments = ["super"], InferModuleQN = "crate::core::resolver", parts = ["crate","core","resolver"]
	//   parts[:2] = ["crate","core"], parentModule = "crate::core"
	//   len(segments)==1, return "crate::core"
	// So modulePath = "crate::core"
	if modulePath != "crate::core" {
		t.Errorf("modulePath = %q, want %q", modulePath, "crate::core")
	}
}

func TestResolveUsePath_External(t *testing.T) {
	modulePath, isExternal := ResolveUsePath("tokio::runtime::Runtime", "src/main.rs")
	if modulePath != "tokio::runtime" {
		t.Errorf("modulePath = %q, want %q", modulePath, "tokio::runtime")
	}
	if !isExternal {
		t.Error("isExternal = false, want true")
	}
}

func TestResolveUsePath_Std(t *testing.T) {
	modulePath, isExternal := ResolveUsePath("std::collections::HashMap", "src/main.rs")
	if modulePath != "std::collections" {
		t.Errorf("modulePath = %q, want %q", modulePath, "std::collections")
	}
	if !isExternal {
		t.Error("isExternal = false, want true")
	}
}

func TestBuildUseMap_Simple(t *testing.T) {
	src := `use crate::core::Config;`
	root := parseRust(t, src)
	uses := BuildUseMap(root, []byte(src), "src/main.rs")

	if got := uses["Config"]; got != "crate::core" {
		t.Errorf("uses[Config] = %q, want %q", got, "crate::core")
	}
}

func TestBuildUseMap_Group(t *testing.T) {
	src := `use crate::core::{Config, Workspace};`
	root := parseRust(t, src)
	uses := BuildUseMap(root, []byte(src), "src/main.rs")

	if got := uses["Config"]; got != "crate::core" {
		t.Errorf("uses[Config] = %q, want %q", got, "crate::core")
	}
	if got := uses["Workspace"]; got != "crate::core" {
		t.Errorf("uses[Workspace] = %q, want %q", got, "crate::core")
	}
}

func TestBuildUseMap_Alias(t *testing.T) {
	src := `use std::io::Read as IoRead;`
	root := parseRust(t, src)
	uses := BuildUseMap(root, []byte(src), "src/main.rs")

	if got := uses["IoRead"]; got != "std::io" {
		t.Errorf("uses[IoRead] = %q, want %q", got, "std::io")
	}
	// Original name should NOT be in the map.
	if _, ok := uses["Read"]; ok {
		t.Error("uses[Read] should not exist for aliased import")
	}
}

func TestBuildUseMap_Glob(t *testing.T) {
	src := `use crate::prelude::*;`
	root := parseRust(t, src)
	uses := BuildUseMap(root, []byte(src), "src/main.rs")

	if len(uses) != 0 {
		t.Errorf("glob import should produce empty map, got %v", uses)
	}
}

func TestInferModuleQN_Regular(t *testing.T) {
	got := InferModuleQN("src/foo/bar.rs")
	if got != "crate::foo::bar" {
		t.Errorf("InferModuleQN(src/foo/bar.rs) = %q, want %q", got, "crate::foo::bar")
	}
}

func TestInferModuleQN_ModRs(t *testing.T) {
	got := InferModuleQN("src/foo/mod.rs")
	if got != "crate::foo" {
		t.Errorf("InferModuleQN(src/foo/mod.rs) = %q, want %q", got, "crate::foo")
	}
}

func TestInferModuleQN_LibRs(t *testing.T) {
	got := InferModuleQN("src/lib.rs")
	if got != "crate" {
		t.Errorf("InferModuleQN(src/lib.rs) = %q, want %q", got, "crate")
	}
}

func TestInferModuleQN_MainRs(t *testing.T) {
	got := InferModuleQN("src/main.rs")
	if got != "crate" {
		t.Errorf("InferModuleQN(src/main.rs) = %q, want %q", got, "crate")
	}
}
