package goextractor

import (
	"context"
	"fmt"
	"go/token"
	"log"

	"golang.org/x/tools/go/packages"
)

// LoadedPackages holds the result of a bulk go/packages.Load("./...") call.
// FilePackages maps absolute file paths to their containing package.
type LoadedPackages struct {
	Fset         *token.FileSet
	FilePackages map[string]*packages.Package // abs path -> package
}

// BulkLoad loads all Go packages under moduleRoot using a single
// go/packages.Load("./...") call. Returns LoadedPackages mapping each
// .go file to its containing package, or an error if the load fails.
func BulkLoad(ctx context.Context, moduleRoot string) (*LoadedPackages, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedModule,
		Dir:     moduleRoot,
		Context: ctx,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("bulk load packages: %w", err)
	}

	result := &LoadedPackages{
		Fset:         cfg.Fset,
		FilePackages: make(map[string]*packages.Package),
	}

	// If no Fset was set on the config, packages.Load creates one internally
	// on each package. We need a consistent Fset, so use the one from the
	// first package if available.
	if result.Fset == nil && len(pkgs) > 0 {
		result.Fset = pkgs[0].Fset
	}

	for _, pkg := range pkgs {
		// Log any package errors without failing the entire load.
		for _, pkgErr := range pkg.Errors {
			log.Printf("package %s: %s", pkg.PkgPath, pkgErr)
		}

		// Map GoFiles (absolute paths) to the package.
		for _, filePath := range pkg.GoFiles {
			result.FilePackages[filePath] = pkg
		}

		// Verify file path association via Syntax AST files.
		for _, f := range pkg.Syntax {
			if pkg.Fset != nil {
				pos := pkg.Fset.Position(f.Pos())
				if pos.Filename != "" {
					// Ensure the syntax file is also in our map.
					if _, ok := result.FilePackages[pos.Filename]; !ok {
						result.FilePackages[pos.Filename] = pkg
					}
				}
			}
		}
	}

	return result, nil
}
