package resolve

import "testing"

func TestInferExternalRepoURL_TypeScript(t *testing.T) {
	tests := []struct {
		modPath string
		want    string
	}{
		// External packages.
		{"react", "external://react"},
		{"@nestjs/common", "external://@nestjs/common"},
		{"lodash/debounce", "external://lodash"},
		{"@scope/pkg/sub", "external://@scope/pkg"},
		{"express", "external://express"},
		{"@types/node", "external://@types/node"},
		{"vue/dist/vue.esm", "external://vue"},

		// Relative imports (not external).
		{"./local", ""},
		{"../relative", ""},
		{"./components/Button", ""},
		{"../utils/format", ""},
		{"/absolute/path", ""},
	}

	for _, tt := range tests {
		got := InferExternalRepoURL(tt.modPath, "", TypeScriptConfig)
		if got != tt.want {
			t.Errorf("InferExternalRepoURL(%q, TypeScript) = %q, want %q", tt.modPath, got, tt.want)
		}
	}
}

func TestInferExternalRepoURL_Python(t *testing.T) {
	tests := []struct {
		moduleName string
		want       string
	}{
		{"flask", "external://flask"},
		{"os", "stdlib"},
		{"numpy.linalg", "external://numpy"},
		{".local_module", ""},
		{"sys", "stdlib"},
		{"requests", "external://requests"},
		{"..parent_module", ""},
		{"django.db.models", "external://django"},
		{"pathlib", "stdlib"},
		{"json", "stdlib"},
		{"typing", "stdlib"},
		{"pandas", "external://pandas"},
	}

	for _, tt := range tests {
		got := InferExternalRepoURL(tt.moduleName, "", PythonConfig)
		if got != tt.want {
			t.Errorf("InferExternalRepoURL(%q, Python) = %q, want %q", tt.moduleName, got, tt.want)
		}
	}
}

func TestInferExternalRepoURL_Rust(t *testing.T) {
	tests := []struct {
		usePath string
		want    string
	}{
		{"tokio::runtime", "external://tokio"},
		{"crate::config", ""},
		{"std::collections", "stdlib"},
		{"serde::Deserialize", "external://serde"},
		{"core::fmt", "stdlib"},
		{"alloc::vec", "stdlib"},
		{"super::helpers", ""},
		{"self::module", ""},
		{"hyper::client::Client", "external://hyper"},
		{"", ""},
	}

	for _, tt := range tests {
		got := InferExternalRepoURL(tt.usePath, "", RustConfig)
		if got != tt.want {
			t.Errorf("InferExternalRepoURL(%q, Rust) = %q, want %q", tt.usePath, got, tt.want)
		}
	}
}

func TestInferExternalRepoURL_Java(t *testing.T) {
	tests := []struct {
		importPath string
		localPkg   string
		want       string
	}{
		// Java stdlib imports return "stdlib".
		{"java.util.List", "com.myapp.service", "stdlib"},
		{"java.util.Map", "", "stdlib"},
		{"javax.servlet.http.HttpServletRequest", "com.myapp.web", "stdlib"},

		// Third-party external packages return "external://{group}".
		{"org.springframework.web.bind.annotation.GetMapping", "com.myapp.service", "external://org.springframework"},
		{"org.apache.commons.lang3.StringUtils", "com.myapp.util", "external://org.apache"},
		{"io.netty.channel.Channel", "com.myapp.net", "external://io.netty"},

		// Same-project imports (first 2 segments match) return "".
		{"com.myapp.model.User", "com.myapp.service", ""},
		{"com.myapp.util.Helper", "com.myapp.controller", ""},

		// Edge cases.
		{"", "", ""},
		{"singleword", "", "external://singleword"},
		{"com.other.Service", "com.myapp.service", "external://com.other"},
	}

	for _, tt := range tests {
		got := InferExternalRepoURL(tt.importPath, tt.localPkg, JavaConfig)
		if got != tt.want {
			t.Errorf("InferExternalRepoURL(%q, %q, Java) = %q, want %q",
				tt.importPath, tt.localPkg, got, tt.want)
		}
	}
}

func TestInferExternalRepoURL_CSharp(t *testing.T) {
	tests := []struct {
		namespace string
		want      string
	}{
		{"System.Collections.Generic", "stdlib"},
		{"System", "stdlib"},
		{"System.IO", "stdlib"},
		{"Microsoft.Extensions.DependencyInjection", "stdlib"},
		{"Microsoft.AspNetCore.Mvc", "stdlib"},
		{"Newtonsoft.Json", "external://Newtonsoft.Json"},
		{"AutoMapper.Extensions", "external://AutoMapper.Extensions"},
		{"Serilog.Sinks.Console", "external://Serilog.Sinks"},
		{"FluentValidation", "external://FluentValidation"},
		{"", ""},
	}

	for _, tt := range tests {
		got := InferExternalRepoURL(tt.namespace, "", CSharpConfig)
		if got != tt.want {
			t.Errorf("InferExternalRepoURL(%q, CSharp) = %q, want %q", tt.namespace, got, tt.want)
		}
	}
}

func TestPythonStdlibSet(t *testing.T) {
	// Positive cases: known stdlib modules.
	stdlibNames := []string{
		"os", "sys", "re", "io", "json", "math", "time", "datetime",
		"collections", "itertools", "functools", "pathlib", "typing",
		"abc", "ast", "asyncio", "logging", "subprocess", "threading",
		"base64", "contextlib", "copy", "csv", "dataclasses",
		"enum", "errno", "glob", "hashlib", "http", "importlib",
		"inspect", "multiprocessing", "operator", "pickle", "platform",
		"pprint", "queue", "random", "shutil", "signal", "socket",
		"sqlite3", "string", "struct", "tempfile", "textwrap",
		"traceback", "unittest", "urllib", "uuid", "warnings",
		"weakref", "xml", "zipfile", "argparse", "configparser",
		"email", "html", "ssl", "secrets", "statistics", "types",
	}
	for _, name := range stdlibNames {
		if !PythonStdlibSet[name] {
			t.Errorf("PythonStdlibSet[%q] = false, want true", name)
		}
	}

	// Negative cases: third-party packages.
	thirdParty := []string{
		"flask", "django", "numpy", "pandas", "requests", "pytest",
		"sqlalchemy", "celery", "boto3", "tensorflow",
	}
	for _, name := range thirdParty {
		if PythonStdlibSet[name] {
			t.Errorf("PythonStdlibSet[%q] = true, want false", name)
		}
	}
}
