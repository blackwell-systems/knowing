package context

// testingEquivalenceClasses returns equivalence classes for testing frameworks.
func testingEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
			Concept:    "TEST_UNIT",
			Phrases:    []string{"unit test", "test case", "test suite", "test class"},
			Targets:    []string{"TestCase", "Test", "TestSuite", "setUp", "tearDown", "BeforeEach", "AfterEach", "beforeAll", "afterAll"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "TEST_MOCK",
			Phrases:    []string{"mock", "mock object", "mock dependency", "stub", "spy", "test double"},
			Targets:    []string{"Mock", "MagicMock", "patch", "mock_calls", "return_value", "side_effect", "Mockito", "when", "verify", "jest.fn", "sinon"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "TEST_FIXTURE",
			Phrases:    []string{"test fixture", "test setup", "test data", "factory", "conftest"},
			Targets:    []string{"fixture", "conftest", "Factory", "FactoryBot", "create", "build", "TestFixture"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "TEST_ASSERT",
			Phrases:    []string{"assertion", "assert equal", "expect", "should equal", "test assertion"},
			Targets:    []string{"assertEqual", "assert", "expect", "Should", "Expect", "require", "assert.Equal", "assert.NoError"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "TEST_INTEGRATION",
			Phrases:    []string{"integration test", "test client", "test server", "end to end", "e2e test"},
			Targets:    []string{"TestClient", "TestServer", "httptest", "NewRequest", "ResponseRecorder", "client", "RequestFactory"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "TEST_BENCHMARK",
			Phrases:    []string{"benchmark", "performance test", "load test", "stress test"},
			Targets:    []string{"Benchmark", "BenchmarkResult", "testing.B", "ResetTimer", "StartTimer", "StopTimer"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
	}
}
