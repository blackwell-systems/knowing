package context

// authEquivalenceClasses returns equivalence classes for authentication/authorization patterns.
func authEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
			Concept:    "AUTH_JWT",
			Phrases:    []string{"jwt token", "json web token", "jwt authentication", "token verification", "decode token"},
			Targets:    []string{"JWT", "JWTAuth", "decode", "encode", "verify", "JWTPayload", "TokenValidator", "Claims"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "AUTH_OAUTH",
			Phrases:    []string{"oauth", "oauth2", "authorization code", "access token", "refresh token", "token endpoint"},
			Targets:    []string{"OAuth2", "OAuth2Client", "authorize", "token", "refresh", "AuthorizationCodeGrant", "AccessToken", "RefreshToken"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "AUTH_SESSION",
			Phrases:    []string{"session management", "session store", "session cookie", "session middleware", "login session"},
			Targets:    []string{"Session", "SessionStore", "SessionMiddleware", "CreateSession", "DestroySession", "GetSession"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "AUTH_RBAC",
			Phrases:    []string{"role based access", "permission check", "access control", "authorize user", "role permission"},
			Targets:    []string{"Role", "Permission", "Authorize", "HasPermission", "RequireRole", "AccessControl", "Policy"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "AUTH_PASSWORD",
			Phrases:    []string{"password hash", "bcrypt", "password validation", "hash password", "verify password"},
			Targets:    []string{"HashPassword", "VerifyPassword", "bcrypt", "CompareHash", "PasswordHasher", "PBKDF2"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
	}
}
