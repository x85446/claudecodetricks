# Fabric Renderer Patterns

## What is Fabric?

Fabric is React Native's new rendering system (part of New Architecture). It replaces the old async bridge-based renderer with synchronous, prioritized rendering.

## Key Benefits

- **Synchronous rendering** - Layout calculated immediately, not async
- **Concurrent features** - Supports React 18 concurrent mode
- **View flattening** - Automatic removal of layout-only views
- **Priority rendering** - User interactions get priority over background updates

## Enabling New Architecture

### iOS
```ruby
# ios/Podfile
ENV['RCT_NEW_ARCH_ENABLED'] = '1'
```

### Android
```properties
# android/gradle.properties
newArchEnabled=true
```

## Fabric-Compatible Components

```typescript
// Fabric requires components to use the new renderer
// Most community libraries already support Fabric:
// ✅ react-native-reanimated
// ✅ react-native-gesture-handler
// ✅ react-native-screens
// ✅ react-native-svg
// ✅ @shopify/flash-list

// Check compatibility:
// https://reactnative.directory/ (filter by New Architecture)
```

## Custom Native Components (Fabric)

```typescript
// JavaScript spec for codegen
// src/specs/MyCustomViewNativeComponent.ts
import type { ViewProps } from 'react-native';
import type { HostComponent } from 'react-native';
import codegenNativeComponent from 'react-native/Libraries/Utilities/codegenNativeComponent';

interface NativeProps extends ViewProps {
  color?: string;
  size?: number;
}

export default codegenNativeComponent<NativeProps>(
  'MyCustomView'
) as HostComponent<NativeProps>;
```

## Migration Checklist

1. Update React Native to 0.73+ (preferably latest)
2. Enable New Architecture in gradle.properties and Podfile
3. Update all dependencies to Fabric-compatible versions
4. Run `npx react-native-new-architecture-helper` to check
5. Test all screens for visual regressions
6. Test animations (should be smoother with Fabric)
7. Test touch handling (synchronous in Fabric)

## Interop Layer

Fabric includes an interop layer for old-architecture components. Most Bridge components work automatically, but may not get Fabric benefits.
