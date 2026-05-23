#!/usr/bin/env bash
# Task definitions for Django (473K LOC Python, 2788 files).
#
# These tasks are designed to test scenarios where grep returns noise:
# - Ambiguous names (View, Manager, Field appear in 100+ files)
# - Multi-hop dependencies (middleware -> view -> model -> manager)
# - Interface-like patterns (class-based views with mixin chains)
# - Discovery across deep package hierarchy

# Django repo path (from corpus)
REPO_PATH="bench/cross-system/corpus/repos/django"
REPO_NAME="django"
VERIFY_CMD="cd \$WORKTREE && python -m py_compile django/db/models/query.py 2>/dev/null; true"

get_prompt_django() {
  case "$1" in
    django-queryset-callers)
      echo 'In the Django codebase, I want to understand what calls QuerySet.filter() in django/db/models/query.py. Find all direct callers of the filter method across the entire codebase. Report: file path, class/function name, and line number for each caller. There are many QuerySet references throughout Django; I need the ones that actually invoke .filter() on a queryset instance.'
      ;;
    django-middleware-chain)
      echo 'Trace the request processing chain in Django. Starting from django/core/handlers/base.py (BaseHandler.get_response), identify: 1) What middleware classes does it invoke? 2) For each middleware, what view-related code does it call? 3) How does a request go from BaseHandler through middleware to a view function? Give me the full call chain with file paths.'
      ;;
    django-field-impact)
      echo 'If I modify the CharField class in django/db/models/fields/__init__.py, what breaks? Find all classes that inherit from CharField, all classes that USE CharField as a field declaration, and any migration-related code that references CharField. Report a dependency tree: direct subclasses, then files that instantiate CharField, then migration utilities that handle it.'
      ;;
    django-admin-registration)
      echo 'How does Django admin site registration work? Trace from admin.site.register() to where the admin views are actually created. I need to understand: 1) Where is admin.site defined? 2) What does register() do internally? 3) How do ModelAdmin classes get their URL patterns wired up? 4) What template/view code renders the admin pages for a registered model?'
      ;;
    django-orm-select-related)
      echo 'I want to understand all the code involved when select_related() is called on a QuerySet. Trace from the select_related() method through to the SQL generation. What functions are involved? Where does it modify the SQL query? How does it handle ForeignKey traversal? Report every function in the chain with file:line.'
      ;;
    django-signal-receivers)
      echo 'Find all signal receivers in the Django codebase. Signals are connected via django/dispatch/dispatcher.py Signal.connect(). I need: 1) Every place in the Django source that calls .connect() on a signal, 2) What signal is being connected to, 3) What receiver function handles it. This requires finding both the signal definitions and their connection points scattered across the codebase.'
      ;;
    *)
      echo ""
      ;;
  esac
}

get_verify_django() {
  # Django tasks are read-only analysis (no edits), so verification is always pass
  echo "true"
}

DJANGO_TASKS="django-queryset-callers django-middleware-chain django-field-impact django-admin-registration django-orm-select-related django-signal-receivers"
