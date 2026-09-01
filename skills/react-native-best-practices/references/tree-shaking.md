# Tree Shaking in React Native

## How Metro Handles Tree Shaking

Metro bundler has limited tree shaking compared to webpack/rollup. Key points:
- Metro does NOT tree-shake by default in all cases
- Hermes helps with dead code elimination at bytecode level
- Direct imports are the most reliable "tree shaking" method

## Strategies

### 1. Direct Imports (Most Reliable)
```typescript
// WRONG - Metro may include entire module
import { debounce } from 'lodash';

// CORRECT - Only includes debounce
import debounce from 'lodash/debounce';
```

### 2. Use Tree-Shakeable Libraries
```typescript
// WRONG - moment is not tree-shakeable
import moment from 'moment';

// CORRECT - date-fns is tree-shakeable
import { format } from 'date-fns';
```

### 3. Conditional Requires
```typescript
// Development-only code stripped in production
if (__DEV__) {
  require('./devTools');
}
// Metro removes this entire block in production builds
```

### 4. Platform-Specific Code
```typescript
// Metro automatically excludes wrong platform
// Component.ios.tsx - Only in iOS bundle
// Component.android.tsx - Only in Android bundle
```

## Verifying Tree Shaking

```bash
# Generate source map and analyze
npx react-native bundle \
  --platform ios --dev false \
  --entry-file index.js \
  --bundle-output /tmp/bundle.js \
  --sourcemap-output /tmp/bundle.js.map

# Use source-map-explorer
npx source-map-explorer /tmp/bundle.js /tmp/bundle.js.map
```

## Metro Configuration

```javascript
// metro.config.js
module.exports = {
  transformer: {
    getTransformOptions: async () => ({
      transform: {
        experimentalImportSupport: true, // Better import handling
        inlineRequires: true, // Defer module loading
      },
    }),
  },
};
```

## `inlineRequires` (Critical)

Converts top-level requires to inline requires, deferring module initialization:

```typescript
// Before inlineRequires
const heavy = require('./heavy'); // Loaded at module init

// After inlineRequires (Metro transforms to)
// require('./heavy') is called only when `heavy` is first used
```

Enable in `metro.config.js` for significant startup improvement.
