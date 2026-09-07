# Eliminating Barrel File Re-Exports

## The Problem

Barrel files (`index.ts` that re-exports) cause the entire module tree to be loaded even if you only need one export.

```typescript
// src/components/index.ts (BARREL FILE)
export { Button } from './atoms/Button';
export { Input } from './atoms/Input';
export { Card } from './molecules/Card';
export { Modal } from './organisms/Modal';
// ... 50 more exports

// WRONG - Importing Button loads ALL components
import { Button } from '../components';
// Metro resolves index.ts → loads every export → huge initial bundle
```

## The Fix

```typescript
// CORRECT - Direct imports
import { Button } from '../components/atoms/Button';
import { Card } from '../components/molecules/Card';

// Each import only loads what's needed
```

## When index.ts is OK

- **Re-exporting a single component** from its directory:
  ```
  components/atoms/Button/
  ├── Button.tsx
  ├── Button.test.tsx
  └── index.ts  ← Re-exports Button only (OK)
  ```

- **NOT OK** for aggregate re-exports:
  ```
  components/
  └── index.ts  ← Re-exports everything (BAD)
  ```

## Detection

```bash
# Find barrel files with many exports
grep -rn "export.*from" src/**/index.ts | wc -l

# Find imports from barrel paths
grep -rn "from '\.\./components'" src/
grep -rn "from '\.\./hooks'" src/
```

## Metro Bundle Analysis

```bash
# Visualize what's in the bundle
npx react-native-bundle-visualizer

# Check for unexpected inclusions
# Large unexpected modules = barrel file pulling in everything
```

## ESLint Rule

```json
{
  "rules": {
    "no-restricted-imports": ["error", {
      "patterns": [{
        "group": ["*/index", "*/index.ts"],
        "message": "Import directly from the module file, not the barrel index."
      }]
    }]
  }
}
```
