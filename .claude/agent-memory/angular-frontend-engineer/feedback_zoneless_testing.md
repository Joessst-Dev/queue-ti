---
name: Zoneless Angular testing patterns
description: Critical testing constraints for this Angular 21 zoneless app — fakeAsync is banned, injection context required for form() calls
type: feedback
---

Do NOT use `fakeAsync`/`tick` in tests — this app uses `provideZonelessChangeDetection()` and zone.js is absent.

**Why:** Angular's `fakeAsync` requires `zone.js/testing`. The app bootstraps with `provideZonelessChangeDetection()` so no zone is loaded. Using `fakeAsync` throws "zone-testing.js is needed for the fakeAsync() test helper".

**How to apply:** Replace all `fakeAsync/tick` patterns with `async/await` + `fixture.whenStable()`. For settling async state after an event: dispatch the event, then `await fixture.whenStable()`, then `fixture.detectChanges()`.

---

`addMetadataRow()` on the Messages component calls `form()` from `@angular/forms/signals`, which internally calls `inject()`. Calling it directly from a test method body (outside a component constructor) throws NG0203.

**Why:** `form()` from `@angular/forms/signals` uses `inject()` internally. Angular's `inject()` must be called from an injection context.

**How to apply:** Wrap calls to `addMetadataRow()` in `TestBed.runInInjectionContext(() => component.addMetadataRow())` in tests.
