package context

// saleorEquivalenceClasses returns equivalence classes for saleor (Django e-commerce).
// These are generalizable e-commerce patterns, not saleor-specific internals.
func saleorEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "ECOMMERCE_CHECKOUT",
		Phrases:    []string{"checkout mutation", "complete checkout", "checkout flow", "create order from checkout", "checkout to order"},
		Targets:    []string{"CheckoutComplete", "OrderCreate", "Checkout", "get_last_active_payment"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "ECOMMERCE_SHIPPING",
		Phrases:    []string{"shipping zone", "shipping method", "shipping pricing", "delivery method", "shipping channel"},
		Targets:    []string{"ShippingZone", "ShippingMethod", "ShippingMethodChannelListing", "is_shipping_required"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "ECOMMERCE_ACCOUNT",
		Phrases:    []string{"account management", "user registration", "account mutation", "customer bulk", "account address"},
		Targets:    []string{"AccountRegister", "AccountDelete", "CustomerBulkDelete", "AccountAddressCreate", "AccountAddressUpdate", "AccountAddressDelete", "AccountRequestDeletion"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "ECOMMERCE_AUTH_BACKEND",
		Phrases:    []string{"authentication backend", "auth backend", "jwt backend", "user permissions", "permission backend"},
		Targets:    []string{"JSONWebTokenBackend", "PluginBackend", "get_all_permissions", "get_permissions", "effective_permissions", "get_user_groups_permissions"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "ECOMMERCE_ASYNC_TASKS",
		Phrases:    []string{"celery task", "background task", "async task", "export task", "order confirmation"},
		Targets:    []string{"ExportTask", "RestrictWriterDBTask", "send_order_confirmation", "send_fulfillment_confirmation"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
	}
}
