module github.com/blackwell-systems/cross-repo-test/module-c

go 1.23

require (
	github.com/blackwell-systems/cross-repo-test/module-a v0.0.0
	github.com/blackwell-systems/cross-repo-test/module-b v0.0.0
)

replace (
	github.com/blackwell-systems/cross-repo-test/module-a => ../module-a
	github.com/blackwell-systems/cross-repo-test/module-b => ../module-b
)
