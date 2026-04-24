---
name: Functional guard testing pattern
description: How to test functional CanActivateFn guards in this project — configure providers per-test, not via overrideProvider
type: feedback
---

Do NOT use `TestBed.overrideProvider` inside `it()` blocks — it throws "Cannot override provider when the test module has already been instantiated" because accessing `Router` (injected in `beforeEach`) already instantiates the module.

**Why:** `overrideProvider` must be called before the TestBed module is instantiated. Injecting any token in a `beforeEach` (e.g. `TestBed.inject(Router)`) instantiates the module for that describe block, making subsequent `overrideProvider` calls illegal.

**How to apply:** For guard tests that need different AuthService behaviour per scenario, call `TestBed.configureTestingModule(...)` fresh inside each `it()` block with the desired service value. Invoke the guard with `TestBed.runInInjectionContext(() => guardFn(...).subscribe(...))`. No `beforeEach` needed.
