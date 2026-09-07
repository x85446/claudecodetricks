# Native Module Development Patterns

## Decision: Community Library vs Custom Module

```
Need native functionality?
├── Check react-native-community libraries
├── Check reactnative.directory
├── Check if New Architecture provides it
└── Only if nothing exists → Write custom module
```

## Turbo Module (New Architecture - Preferred)

### 1. TypeScript Spec
```typescript
// src/specs/NativeDeviceInfo.ts
import type { TurboModule } from 'react-native';
import { TurboModuleRegistry } from 'react-native';

export interface Spec extends TurboModule {
  getDeviceId(): string;           // Synchronous
  getBatteryLevel(): Promise<number>; // Async
  getConstants(): {
    appVersion: string;
    buildNumber: string;
  };
}

export default TurboModuleRegistry.getEnforcing<Spec>('DeviceInfo');
```

### 2. Run Codegen
```bash
npx react-native codegen
```

### 3. Implement iOS (Swift)
```swift
@objc(DeviceInfo)
class DeviceInfo: NSObject, NativeDeviceInfoSpec {
  @objc func getDeviceId() -> String {
    return UIDevice.current.identifierForVendor?.uuidString ?? ""
  }

  @objc func getBatteryLevel(_ resolve: @escaping RCTPromiseResolveBlock,
                              reject: @escaping RCTPromiseRejectBlock) {
    UIDevice.current.isBatteryMonitoringEnabled = true
    resolve(UIDevice.current.batteryLevel)
  }

  @objc func getConstants() -> [String: Any] {
    return [
      "appVersion": Bundle.main.infoDictionary?["CFBundleShortVersionString"] ?? "",
      "buildNumber": Bundle.main.infoDictionary?["CFBundleVersion"] ?? ""
    ]
  }
}
```

### 4. Implement Android (Kotlin)
```kotlin
class DeviceInfoModule(reactContext: ReactApplicationContext) :
    NativeDeviceInfoSpec(reactContext) {

  override fun getName() = "DeviceInfo"

  override fun getDeviceId(): String {
    return Settings.Secure.getString(
      reactApplicationContext.contentResolver,
      Settings.Secure.ANDROID_ID
    )
  }

  override fun getBatteryLevel(promise: Promise) {
    val bm = reactApplicationContext.getSystemService(Context.BATTERY_SERVICE)
      as BatteryManager
    val level = bm.getIntProperty(BatteryManager.BATTERY_PROPERTY_CAPACITY)
    promise.resolve(level.toDouble())
  }

  override fun getConstants(): Map<String, Any> {
    val info = reactApplicationContext.packageManager
      .getPackageInfo(reactApplicationContext.packageName, 0)
    return mapOf(
      "appVersion" to (info.versionName ?: ""),
      "buildNumber" to info.versionCode.toString()
    )
  }
}
```

## Bridge Module (Legacy - Still Works)

For backward compatibility or simpler needs:

### iOS Bridge (Objective-C)
```objc
// MyModule.m
#import <React/RCTBridgeModule.h>

@interface RCT_EXTERN_MODULE(MyModule, NSObject)
RCT_EXTERN_METHOD(doSomething:(NSString *)input
                  resolve:(RCTPromiseResolveBlock)resolve
                  reject:(RCTPromiseRejectBlock)reject)
@end
```

### Android Bridge
```kotlin
class MyModulePackage : ReactPackage {
  override fun createNativeModules(reactContext: ReactApplicationContext) =
    listOf(MyModule(reactContext))

  override fun createViewManagers(reactContext: ReactApplicationContext) =
    emptyList<ViewManager<*, *>>()
}
```

## Testing Native Modules

```typescript
// __mocks__/react-native-modules.ts
jest.mock('react-native', () => ({
  ...jest.requireActual('react-native'),
  NativeModules: {
    DeviceInfo: {
      getDeviceId: jest.fn(() => 'test-device-id'),
      getBatteryLevel: jest.fn(() => Promise.resolve(0.85)),
      getConstants: jest.fn(() => ({
        appVersion: '1.0.0',
        buildNumber: '1',
      })),
    },
  },
}));
```
