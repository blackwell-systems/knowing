package goextractor

import (
	"context"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// LoadedPackages holds the result of bulk package loading. It provides
// a mapping from absolute file paths to their containing *packages.Package,
// allowing the indexer to call ExtractWithPackage for each file without
// re-invoking the Go compiler.
type LoadedPackages struct {
	Fset         *token.FileSet                // shared file set across all loaded packages
	FilePackages map[string]*packages.Package // absolute file path -> containing package
}

// BulkLoad loads all Go packages under moduleRoot by discovering unique
// package directories and loading each one individually. This is faster
// than packages.Load("./...") for large repos because Go's build cache
// handles shared dependency type info, and we avoid loading test packages
// and transitive dependencies in a single massive type-check pass.
func BulkLoad(ctx context.Context, moduleRoot string) (*LoadedPackages, error) {
	// Discover unique package directories by walking for .go files.
	pkgDirs := discoverPackageDirs(moduleRoot)
	if len(pkgDirs) == 0 {
		return &LoadedPackages{
			Fset:         token.NewFileSet(),
			FilePackages: make(map[string]*packages.Package),
		}, nil
	}

	fset := token.NewFileSet()
	result := &LoadedPackages{
		Fset:         fset,
		FilePackages: make(map[string]*packages.Package),
	}

	// Load each package directory individually rather than using "./...".
	// Go's build cache ensures shared dependency type info is computed once
	// and reused across packages, so per-directory loading avoids the memory
	// spike of loading the entire module graph in a single pass.
	for _, dir := range pkgDirs {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		cfg := &packages.Config{
			Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
				packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
				packages.NeedModule,
			Dir:     dir,
			Fset:    fset,
			Context: ctx,
		}

		pkgs, err := packages.Load(cfg, ".")
		if err != nil {
			log.Printf("load package in %s: %v", dir, err)
			continue
		}

		for _, pkg := range pkgs {
			for _, pkgErr := range pkg.Errors {
				log.Printf("package %s: %s", pkg.PkgPath, pkgErr)
			}

			for _, filePath := range pkg.GoFiles {
				result.FilePackages[filePath] = pkg
			}

			for _, f := range pkg.Syntax {
				if pkg.Fset != nil {
					pos := pkg.Fset.Position(f.Pos())
					if pos.Filename != "" {
						if _, ok := result.FilePackages[pos.Filename]; !ok {
							result.FilePackages[pos.Filename] = pkg
						}
					}
				}
			}
		}
	}

	return result, nil
}

// discoverPackageDirs walks moduleRoot and returns sorted unique directories
// containing .go files (excluding vendor, .git, node_modules, test files).
func discoverPackageDirs(root string) []string {
	seen := make(map[string]struct{})

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" || name == ".claude" {
				return filepath.SkipDir
			}
			// Skip hidden directories
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			dir := filepath.Dir(path)
			seen[dir] = struct{}{}
		}
		return nil
	})

	dirs := make([]string, 0, len(seen))
	for dir := range seen {
		// Skip testdata directories
		if strings.Contains(dir, string(os.PathSeparator)+"testdata"+string(os.PathSeparator)) ||
			strings.HasSuffix(dir, string(os.PathSeparator)+"testdata") {
			continue
		}
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}
