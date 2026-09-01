# Deep Linking Setup (Bare React Native)

## React Navigation Configuration

```typescript
// src/navigation/linking.ts
import { LinkingOptions } from '@react-navigation/native';
import { RootStackParamList } from './types';

export const linking: LinkingOptions<RootStackParamList> = {
  prefixes: ['gravhl://', 'https://gravhl.com'],
  config: {
    screens: {
      MainTabs: {
        screens: {
          Home: '',
          Projects: 'projects',
          Inbox: 'inbox',
          Profile: 'profile',
        },
      },
      ProjectDetail: 'project/:projectId',
      BidDetail: 'bid/:bidId',
      Chat: 'chat/:conversationId',
      Settings: 'settings',
    },
  },
};
```

## iOS Setup

### URL Scheme (Custom)
```xml
<!-- ios/MyApp/Info.plist -->
<key>CFBundleURLTypes</key>
<array>
  <dict>
    <key>CFBundleURLSchemes</key>
    <array>
      <string>gravhl</string>
    </array>
  </dict>
</array>
```

### Universal Links (HTTPS)
```json
// .well-known/apple-app-site-association (on your server)
{
  "applinks": {
    "details": [{
      "appID": "TEAM_ID.com.gravhl.mobile",
      "paths": ["/project/*", "/bid/*", "/chat/*", "/profile/*"]
    }]
  }
}
```

```xml
<!-- ios/MyApp/MyApp.entitlements -->
<key>com.apple.developer.associated-domains</key>
<array>
  <string>applinks:gravhl.com</string>
</array>
```

### AppDelegate
```swift
// ios/MyApp/AppDelegate.swift
func application(_ app: UIApplication, open url: URL, options: ...) -> Bool {
  return RCTLinkingManager.application(app, open: url, options: options)
}

func application(_ application: UIApplication,
                 continue userActivity: NSUserActivity,
                 restorationHandler: ...) -> Bool {
  return RCTLinkingManager.application(application, continue: userActivity,
                                        restorationHandler: restorationHandler)
}
```

## Android Setup

### URL Scheme
```xml
<!-- android/app/src/main/AndroidManifest.xml -->
<activity android:name=".MainActivity" android:launchMode="singleTask">
  <!-- Custom scheme -->
  <intent-filter>
    <action android:name="android.intent.action.VIEW" />
    <category android:name="android.intent.category.DEFAULT" />
    <category android:name="android.intent.category.BROWSABLE" />
    <data android:scheme="gravhl" />
  </intent-filter>

  <!-- App Links (HTTPS) -->
  <intent-filter android:autoVerify="true">
    <action android:name="android.intent.action.VIEW" />
    <category android:name="android.intent.category.DEFAULT" />
    <category android:name="android.intent.category.BROWSABLE" />
    <data android:scheme="https" android:host="gravhl.com" />
  </intent-filter>
</activity>
```

## Testing Deep Links

```bash
# iOS Simulator
xcrun simctl openurl booted "gravhl://project/123"
xcrun simctl openurl booted "https://gravhl.com/project/123"

# Android Emulator
adb shell am start -W -a android.intent.action.VIEW \
  -d "gravhl://project/123" com.gravhl.mobile
```

## Handling in App

```typescript
import { Linking } from 'react-native';

// Listen for deep links while app is running
useEffect(() => {
  const subscription = Linking.addEventListener('url', ({ url }) => {
    console.log('Deep link received:', url);
    // React Navigation handles routing automatically via linking config
  });
  return () => subscription.remove();
}, []);

// Check for initial deep link (app opened via link)
useEffect(() => {
  Linking.getInitialURL().then(url => {
    if (url) console.log('Opened via deep link:', url);
  });
}, []);
```
