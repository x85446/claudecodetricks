# Bare React Native CLI Development

Custom skill for React Native CLI (NO EXPO) development patterns. Covers CLI commands, Metro bundler, Hermes, native module development, and production patterns from real bare-RN apps.

**CRITICAL: This project uses bare React Native CLI. NEVER suggest Expo, expo-router, EAS Build, or any expo-* package.**

## When to Reference

- Metro bundler issues or configuration
- React Native CLI commands
- Native module development (Swift/Kotlin)
- Build and deployment workflows
- Hermes engine specifics

## CLI Commands Quick Reference

### Development
```bash
# Start Metro bundler
npx react-native start
npx react-native start --reset-cache     # Clear Metro cache

# Run on devices/simulators
npx react-native run-ios
npx react-native run-ios --device "iPhone 15 Pro"
npx react-native run-ios --simulator "iPhone 15 Pro"
npx react-native run-android

# Logging
npx react-native log-ios
npx react-native log-android

# Bundle analysis
npx react-native-bundle-visualizer

# Codegen (New Architecture)
npx react-native codegen
```

### Build
```bash
# iOS Release
npx react-native run-ios --mode Release
# Or via Xcode: Product → Archive

# Android Release
cd android && ./gradlew assembleRelease
cd android && ./gradlew bundleRelease  # AAB for Play Store

# Clean builds
cd ios && xcodebuild clean && pod install
cd android && ./gradlew clean
watchman watch-del-all
```

### Troubleshooting
```bash
# Nuclear clean
watchman watch-del-all
rm -rf node_modules && npm install
cd ios && pod install --repo-update
cd android && ./gradlew clean

# Check Hermes
adb shell getprop | grep hermes  # Android

# Check port
lsof -i :8081  # Metro port

# Kill stale Metro
lsof -ti:8081 | xargs kill -9
```

## Metro Bundler Configuration

```javascript
// metro.config.js
const { getDefaultConfig, mergeConfig } = require('@react-native/metro-config');

const defaultConfig = getDefaultConfig(__dirname);

const config = {
  transformer: {
    getTransformOptions: async () => ({
      transform: {
        experimentalImportSupport: true,
        inlineRequires: true, // Critical for startup performance
      },
    }),
  },
  resolver: {
    // Add custom asset extensions
    assetExts: [...defaultConfig.resolver.assetExts, 'lottie'],
    // Block list for files Metro should ignore
    blockList: [
      /.*\.test\.tsx?$/,  // Don't bundle test files
    ],
  },
};

module.exports = mergeConfig(defaultConfig, config);
```

## Hermes Engine

### Verification
```bash
# Check if Hermes is enabled
# In app JS:
const isHermes = () => !!global.HermesInternal;
console.log('Hermes enabled:', isHermes());
```

### Hermes Compatibility
- No `eval()` or `new Function()`
- Limited `Proxy` support (improved in newer versions)
- Intl support varies (may need polyfills)
- No `with` statement
- Source maps work with Chrome DevTools (not Safari)

## React Navigation (Bare RN Setup)

```bash
# Required dependencies (no expo packages!)
npm install @react-navigation/native @react-navigation/native-stack
npm install react-native-screens react-native-safe-area-context

# iOS pods
cd ios && pod install
```

```typescript
// index.js or App.tsx - Required for react-native-screens
import { enableScreens } from 'react-native-screens';
enableScreens();
```

## Zustand + MMKV Pattern

```typescript
// stores/authStore.ts
import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import { MMKV } from 'react-native-mmkv';

const storage = new MMKV();

const mmkvStorage = {
  getItem: (name: string) => {
    const value = storage.getString(name);
    return value ?? null;
  },
  setItem: (name: string, value: string) => {
    storage.set(name, value);
  },
  removeItem: (name: string) => {
    storage.delete(name);
  },
};

interface AuthState {
  frontToken: string | null;
  setFrontToken: (token: string | null) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      frontToken: null,
      setFrontToken: (token) => set({ frontToken: token }),
      logout: () => set({ frontToken: null }),
    }),
    {
      name: 'auth-storage',
      storage: createJSONStorage(() => mmkvStorage),
    }
  )
);
```

## React Query Pattern

```typescript
// hooks/useProjects.ts
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../lib/api/client';

export const useProjects = () => {
  return useQuery({
    queryKey: ['projects'],
    queryFn: async () => {
      const response = await apiClient.get('/projects');
      return response.data.data; // Double-nested: response.data.data
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
};

export const useCreateProject = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: CreateProjectInput) => {
      const response = await apiClient.post('/projects', data);
      return response.data.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
    },
  });
};
```

## Native Module Development

### iOS (Swift)
```swift
// ios/MyModule/MyModule.swift
import Foundation

@objc(MyModule)
class MyModule: NSObject {
  @objc
  func multiply(_ a: Double, b: Double, resolve: @escaping RCTPromiseResolveBlock, reject: @escaping RCTPromiseRejectBlock) {
    resolve(a * b)
  }

  @objc
  static func requiresMainQueueSetup() -> Bool {
    return false
  }
}
```

### Android (Kotlin)
```kotlin
// android/app/src/main/java/com/myapp/MyModule.kt
package com.myapp

import com.facebook.react.bridge.*

class MyModule(reactContext: ReactApplicationContext) :
    ReactContextBaseJavaModule(reactContext) {

    override fun getName() = "MyModule"

    @ReactMethod
    fun multiply(a: Double, b: Double, promise: Promise) {
        promise.resolve(a * b)
    }
}
```

## Build & Deployment (Fastlane)

```ruby
# ios/fastlane/Fastfile
platform :ios do
  lane :beta do
    increment_build_number
    build_app(scheme: "MyApp", workspace: "MyApp.xcworkspace")
    upload_to_testflight
  end
end

# android/fastlane/Fastfile
platform :android do
  lane :beta do
    gradle(task: "bundleRelease")
    upload_to_play_store(track: "beta")
  end
end
```

## References
- `references/metro-config.md` - Advanced Metro configuration
- `references/native-modules.md` - Detailed native module patterns
- `references/deep-linking.md` - Deep linking setup
- `references/push-notifications.md` - Push notification setup
- `references/ci-cd.md` - CI/CD pipeline configuration
