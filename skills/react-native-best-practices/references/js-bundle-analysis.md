# JS Bundle Analysis

## Quick Analysis

```bash
# Visualize bundle composition
npx react-native-bundle-visualizer

# Generate bundle for analysis
npx react-native bundle \
  --platform ios \
  --dev false \
  --entry-file index.js \
  --bundle-output /tmp/bundle.js \
  --sourcemap-output /tmp/bundle.js.map

# Check bundle size
ls -lh /tmp/bundle.js
```

## Budget: <2MB JS Bundle

| Category | Budget | Common Offenders |
|----------|--------|-----------------|
| App code | <500KB | Over-engineering, dead code |
| React + RN | ~600KB | Fixed cost |
| Navigation | ~100KB | Fixed cost |
| State (Zustand) | ~10KB | Lean choice |
| Utilities | <100KB | lodash (full), moment.js |
| UI libraries | <200KB | Heavy component libs |
| **Total** | **<2MB** | |

## Common Bundle Bloaters

| Library | Size | Alternative | Savings |
|---------|------|-------------|---------|
| moment.js | ~300KB | date-fns (tree-shakeable) | ~280KB |
| lodash (full) | ~70KB | lodash-es or direct imports | ~60KB |
| i18next (all locales) | Variable | Lazy-load locales | 50-90% |
| axios | ~30KB | fetch (built-in) | ~30KB |

## Direct Import Pattern

```typescript
// WRONG - Imports entire library
import _ from 'lodash';
_.debounce(fn, 300);

// CORRECT - Tree-shakeable import
import debounce from 'lodash/debounce';
debounce(fn, 300);
```

## Hermes Bytecode

Hermes pre-compiles JS to bytecode (.hbc), which:
- Eliminates parse time at startup
- Reduces memory for source code
- Bundle on disk may be larger but runtime is smaller

```bash
# Check if Hermes bytecode is being used
# The bundle should be .hbc format in release builds
# Verify with:
file android/app/build/generated/assets/createBundleReleaseJsAndAssets/index.android.bundle
# Should show: "Hermes JavaScript bytecode"
```

## Monitoring Over Time

Track bundle size in CI:
```bash
# In CI pipeline
npx react-native bundle --platform ios --dev false \
  --entry-file index.js --bundle-output /tmp/bundle.js
SIZE=$(wc -c < /tmp/bundle.js)
echo "Bundle size: $SIZE bytes"
# Fail if over budget
if [ $SIZE -gt 2097152 ]; then echo "OVER 2MB BUDGET!"; exit 1; fi
```
