# Turbo Modules (New Architecture)

## What Are Turbo Modules?

Turbo Modules replace the old Bridge-based Native Modules with synchronous, type-safe, lazy-loaded native code access.

## Key Benefits

- **Lazy loading** - Modules loaded on first use, not at startup
- **Synchronous calls** - No bridge serialization overhead
- **Type-safe** - Codegen from TypeScript specs
- **JSI-based** - Direct memory access between JS and Native

## When to Use

| Scenario | Old Bridge | Turbo Module |
|----------|-----------|--------------|
| Startup modules | Loaded eagerly | Lazy loaded |
| Frequent calls | Bridge overhead | Near-zero overhead |
| Large data | Serialization cost | Direct memory |
| Type safety | Runtime errors | Compile-time |

## Checking If Available

```typescript
// Check if app uses New Architecture
import { TurboModuleRegistry } from 'react-native';

const isTurboModuleAvailable = TurboModuleRegistry.get('MyModule') !== null;
```

## Creating a Turbo Module Spec

```typescript
// src/specs/NativeMyModule.ts
import type { TurboModule } from 'react-native';
import { TurboModuleRegistry } from 'react-native';

export interface Spec extends TurboModule {
  multiply(a: number, b: number): number; // Synchronous!
  fetchData(url: string): Promise<string>; // Async
  getConstants(): {
    version: string;
    platform: string;
  };
}

export default TurboModuleRegistry.getEnforcing<Spec>('MyModule');
```

## Migration Strategy

1. Check if community library already supports New Architecture
2. If custom native module, create Turbo Module spec
3. Run codegen: `npx react-native codegen`
4. Implement native side (iOS Swift/ObjC, Android Kotlin/Java)
5. Test with New Architecture enabled

## Community Library Status

Most major libraries support New Architecture:
- react-native-reanimated ✅
- react-native-gesture-handler ✅
- react-native-screens ✅
- react-native-svg ✅
- react-native-fast-image ⚠️ (check latest)

Check: `npx react-native-new-architecture-helper`
