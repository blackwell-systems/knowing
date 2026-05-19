module github.com/blackwell-systems/cross-repo-test/module-b

go 1.23

require github.com/blackwell-systems/cross-repo-test/module-a v0.0.0

replace github.com/blackwell-systems/cross-repo-test/module-a => ../module-a
