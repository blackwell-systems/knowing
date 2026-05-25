package context

import "strings"

// conceptThesaurus maps programming domain terms to related code vocabulary.
// When a task description contains a key, its related terms are added as
// supplemental BM25 search terms. This bridges the vocabulary gap between
// how developers describe tasks and how code names its symbols.
//
// Design constraints (from session 14 failure analysis):
// - Only add terms that appear as actual symbol/package names in real codebases
// - Expansions must be specific enough to not flood BM25 with noise
// - Each entry limited to 5 related terms (prevents dilution)
// - Terms are lowercase (BM25 is case-insensitive)
var conceptThesaurus = map[string][]string{
	// Database / persistence
	"migration":  {"schema", "migrate", "alembic", "flyway", "ALTER"},
	"database":   {"query", "repository", "persistence", "transaction", "dialect"},
	"query":      {"repository", "finder", "criteria", "specification", "predicate"},
	"repository": {"store", "persistence", "finder", "gateway", "datasource"},
	"transaction": {"commit", "rollback", "isolation", "savepoint", "atomic"},

	// Web / HTTP
	"middleware": {"handler", "interceptor", "filter", "pipeline", "chain"},
	"handler":    {"controller", "endpoint", "route", "dispatch", "servlet"},
	"endpoint":   {"handler", "controller", "route", "resource", "servlet"},
	"route":      {"handler", "endpoint", "dispatch", "mapping", "controller"},
	"controller": {"handler", "endpoint", "resource", "action", "servlet"},
	"request":    {"handler", "middleware", "interceptor", "filter", "context"},
	"response":   {"renderer", "serializer", "formatter", "writer", "template"},
	"session":    {"cookie", "token", "storage", "middleware", "authentication"},

	// Authentication / security
	"authentication": {"login", "credential", "token", "session", "provider"},
	"authorization":  {"permission", "policy", "guard", "role", "access"},
	"token":          {"jwt", "bearer", "refresh", "session", "credential"},
	"permission":     {"role", "policy", "guard", "access", "authorization"},

	// Messaging / events
	"consumer":  {"subscriber", "listener", "handler", "processor", "receiver"},
	"producer":  {"publisher", "emitter", "sender", "dispatcher", "writer"},
	"subscriber": {"consumer", "listener", "handler", "observer", "watcher"},
	"publisher": {"producer", "emitter", "broadcaster", "dispatcher", "sender"},
	"event":     {"listener", "handler", "emitter", "dispatcher", "observer"},
	"queue":     {"consumer", "producer", "broker", "channel", "buffer"},

	// Concurrency
	"scheduler":  {"dispatcher", "executor", "coordinator", "planner", "worker"},
	"worker":     {"executor", "processor", "runner", "consumer", "goroutine"},
	"coordinator": {"orchestrator", "scheduler", "manager", "director", "leader"},
	"pool":       {"worker", "executor", "limiter", "semaphore", "throttle"},

	// Serialization / encoding
	"serializer": {"encoder", "marshaler", "formatter", "converter", "codec"},
	"parser":     {"decoder", "unmarshaler", "lexer", "tokenizer", "reader"},
	"encoder":    {"serializer", "marshaler", "writer", "formatter", "codec"},
	"decoder":    {"parser", "unmarshaler", "reader", "deserializer", "codec"},

	// Validation
	"validator":  {"checker", "verifier", "constraint", "assertion", "sanitizer"},
	"validation": {"constraint", "rule", "schema", "assertion", "sanitize"},

	// Caching
	"cache":   {"store", "invalidation", "eviction", "ttl", "memoize"},
	"eviction": {"cache", "expiration", "ttl", "policy", "cleanup"},

	// Testing
	"mock":    {"stub", "fake", "spy", "fixture", "double"},
	"fixture": {"factory", "builder", "seed", "sample", "testdata"},

	// Configuration
	"config":      {"settings", "options", "properties", "environment", "provider"},
	"environment": {"config", "variable", "settings", "profile", "context"},

	// Lifecycle
	"initialize": {"bootstrap", "startup", "setup", "configure", "register"},
	"shutdown":   {"cleanup", "teardown", "dispose", "close", "finalize"},
	"lifecycle":  {"startup", "shutdown", "initialize", "dispose", "hook"},

	// Error handling
	"retry":     {"backoff", "circuit", "resilience", "fallback", "timeout"},
	"circuit":   {"breaker", "retry", "fallback", "resilience", "bulkhead"},
	"fallback":  {"default", "retry", "recovery", "degradation", "backup"},

	// Patterns
	"factory":   {"builder", "creator", "provider", "constructor", "supplier"},
	"builder":   {"factory", "constructor", "assembler", "creator", "fluent"},
	"observer":  {"listener", "subscriber", "watcher", "callback", "hook"},
	"adapter":   {"wrapper", "bridge", "converter", "decorator", "proxy"},
	"decorator": {"wrapper", "middleware", "interceptor", "enhancer", "proxy"},
	"proxy":     {"delegate", "wrapper", "interceptor", "stub", "adapter"},

	// Networking
	"connection": {"socket", "client", "transport", "dialer", "pool"},
	"client":     {"connection", "transport", "requester", "sender", "dialer"},
	"server":     {"listener", "handler", "acceptor", "dispatcher", "service"},
	"transport":  {"connection", "client", "protocol", "dialer", "channel"},

	// Storage
	"storage":  {"backend", "driver", "provider", "adapter", "persistence"},
	"provider": {"backend", "driver", "factory", "supplier", "adapter"},
}

// expandKeywords adds related code vocabulary terms for each keyword that
// appears in the concept thesaurus. Returns at most maxExpansions new terms.
func expandKeywords(keywords []string, maxExpansions int) []string {
	var expansions []string
	seen := make(map[string]bool)
	for _, kw := range keywords {
		seen[kw] = true
	}

	for _, kw := range keywords {
		related, ok := conceptThesaurus[strings.ToLower(kw)]
		if !ok {
			continue
		}
		for _, r := range related {
			if seen[r] {
				continue
			}
			seen[r] = true
			expansions = append(expansions, r)
			if len(expansions) >= maxExpansions {
				return expansions
			}
		}
	}
	return expansions
}
