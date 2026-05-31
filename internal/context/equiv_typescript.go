package context

// typescriptEquivalenceClasses returns equivalence classes for typescript.
func typescriptEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "TS_COMPONENT",
		Phrases:    []string{"component", "ui", "render", "view", "widget", "element"},
		Targets:    []string{"Component", "FC", "Props", "useState", "useEffect", "useRef", "useMemo", "useCallback", "render", "createElement"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "TS_STATE",
		Phrases:    []string{"state", "store", "redux", "state management", "reducer", "action"},
		Targets:    []string{"store", "createStore", "reducer", "action", "dispatch", "useSelector", "useDispatch", "createSlice", "configureStore", "Provider"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "TS_ROUTING",
		Phrases:    []string{"route", "router", "navigation", "page", "link"},
		Targets:    []string{"Router", "Route", "Switch", "useRouter", "useNavigate", "useParams", "Link", "NavLink", "createBrowserRouter", "Outlet"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "TS_API",
		Phrases:    []string{"api", "fetch", "request", "http", "client", "endpoint"},
		Targets:    []string{"fetch", "axios", "useSWR", "useQuery", "useMutation", "createApi", "client", "request", "response", "interceptor"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "TS_VALIDATION",
		Phrases:    []string{"validation", "schema", "type check", "parse", "transform"},
		Targets:    []string{"schema", "validate", "parse", "safeParse", "transform", "refine", "z.object", "z.string", "yup", "Joi"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "TS_TYPE_SYSTEM",
		Phrases:    []string{"type", "interface", "generic", "type parameter", "inference", "narrowing", "declaration"},
		Targets:    []string{"Type", "TypeNode", "TypeChecker", "checker", "getTypeOfSymbol", "isTypeAssignableTo", "createType", "getSignatureFromDeclaration", "resolveSignature", "inferTypeArguments"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "TS_COMPILER",
		Phrases:    []string{"compiler", "emit", "transform", "visitor", "node", "ast", "parse", "scanner"},
		Targets:    []string{"Transformer", "Visitor", "visitNode", "visitEachChild", "createSourceFile", "emitFiles", "transformNodes", "Scanner", "Parser", "createPrinter"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "TS_MODULE",
		Phrases:    []string{"module", "import", "export", "resolution", "bundle", "package"},
		Targets:    []string{"resolveModuleName", "resolveModuleNames", "getResolvedModule", "moduleSpecifierToSourceFile", "createModuleSpecifierResolutionHost", "NodeModulePathResolver"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
	}
}
