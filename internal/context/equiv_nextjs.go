package context

// nextjsEquivalenceClasses returns equivalence classes for nextjs.
func nextjsEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "NEXTJS_SSR",
		Phrases:    []string{"server-side rendering", "getServerSideProps", "server component", "ssr"},
		Targets:    []string{"getServerSideProps", "getStaticProps", "getStaticPaths", "generateStaticParams", "generateMetadata"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "NEXTJS_MIDDLEWARE",
		Phrases:    []string{"nextjs middleware", "edge middleware", "next middleware", "request rewrite"},
		Targets:    []string{"middleware", "NextRequest", "NextResponse", "NextMiddleware"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "NEXTJS_API",
		Phrases:    []string{"api route", "api handler", "next api", "route handler"},
		Targets:    []string{"NextApiRequest", "NextApiResponse", "NextRequest", "NextResponse", "GET", "POST"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
	}
}
