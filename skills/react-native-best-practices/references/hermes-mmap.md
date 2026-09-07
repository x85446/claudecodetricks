# Hermes Bytecode & Memory Mapping

## How Hermes Works

Hermes compiles JavaScript to bytecode (.hbc) at build time:
1. **Build time**: JS → Hermes bytecode (.hbc)
2. **Runtime**: .hbc loaded via memory mapping (mmap)
3. **Benefit**: No parse/compile at startup, reduced memory

## Why Memory Mapping Matters

Traditional JS engines:
- Parse source → AST → Bytecode → Execute
- All steps happen at runtime, consuming CPU and memory

Hermes:
- Bytecode pre-compiled at build time
- mmap loads bytecode directly from disk
- Pages loaded on demand (not all at once)
- Shared across processes (if applicable)
- **Result**: Faster startup, lower memory footprint

## Verification

```bash
# Android: Check Hermes is producing bytecode
file android/app/build/generated/assets/createBundleReleaseJsAndAssets/index.android.bundle
# Should output: "Hermes JavaScript bytecode, version XX"

# iOS: Check Hermes compilation
# In Xcode build log, look for "Compiling JS to Hermes bytecode"
```

## Hermes Optimization Tips

### DO
- Use standard JavaScript patterns (Hermes optimizes well for them)
- Keep functions small (better inline optimization)
- Use typed patterns (Hermes benefits from predictable types)
- Use `for` loops over `forEach` for hot paths

### DON'T
- Use `eval()` or `new Function()` - Hermes can't pre-compile these
- Use `with` statement - not supported
- Rely on `Proxy` for hot paths - some overhead
- Use WeakRef extensively - limited support in older Hermes

## Hermes vs JSC Performance

| Metric | Hermes | JSC (JavaScriptCore) |
|--------|--------|---------------------|
| Startup time | 30-50% faster | Baseline |
| Memory (idle) | 20-30% less | Baseline |
| Parse time | ~0 (pre-compiled) | Significant |
| Peak execution | Similar | Similar |
| Intl support | Partial (growing) | Full |
| Debugger | Chrome DevTools | Safari DevTools |

## Enabling Hermes

```ruby
# ios/Podfile
:hermes_enabled => true
```

```groovy
// android/app/build.gradle
project.ext.react = [
    enableHermes: true,
]
```

```bash
# After enabling, clean and rebuild
cd ios && pod install
cd android && ./gradlew clean
```
