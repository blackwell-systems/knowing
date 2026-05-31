package context

// ormEquivalenceClasses returns equivalence classes for ORM/database patterns.
func ormEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
			Concept:    "ORM_QUERY",
			Phrases:    []string{"database query", "query builder", "query set", "find records", "select query"},
			Targets:    []string{"QuerySet", "Query", "QueryBuilder", "Select", "Where", "Find", "FindAll", "filter", "exclude"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "ORM_RELATION",
			Phrases:    []string{"model relationship", "foreign key", "one to many", "many to many", "association", "join table"},
			Targets:    []string{"ForeignKey", "ManyToManyField", "OneToOneField", "has_many", "belongs_to", "has_one", "JoinColumn", "ManyToOne", "OneToMany"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "ORM_MIGRATION",
			Phrases:    []string{"database migration", "schema change", "alter table", "add column", "migration file"},
			Targets:    []string{"Migration", "migrate", "Schema", "CreateTable", "AddColumn", "AlterColumn", "DropColumn", "RunMigrations"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "ORM_TRANSACTION",
			Phrases:    []string{"database transaction", "commit", "rollback", "atomic", "transaction scope"},
			Targets:    []string{"Transaction", "Begin", "Commit", "Rollback", "atomic", "transaction", "SavePoint"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "ORM_CONNECTION",
			Phrases:    []string{"database connection", "connection pool", "connection string", "database url"},
			Targets:    []string{"Connection", "ConnectionPool", "connect", "create_engine", "DatabaseURL", "DataSource"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
	}
}
