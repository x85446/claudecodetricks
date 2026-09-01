# Push Notification Setup (Bare React Native)

## Library: react-native-push-notification or @notifee/react-native

```bash
# Notifee (recommended - from Invertase/Firebase team)
npm install @notifee/react-native

# iOS
cd ios && pod install
```

## iOS Setup

### Capabilities
1. Xcode → Target → Signing & Capabilities
2. Add "Push Notifications"
3. Add "Background Modes" → "Remote notifications"

### APNs Key
1. Apple Developer → Keys → Create new key
2. Enable "Apple Push Notifications service (APNs)"
3. Download .p8 file
4. Note: Key ID, Team ID

## Android Setup

### Firebase (for FCM)
1. Firebase Console → Add Android app
2. Download `google-services.json` → `android/app/`
3. Configure gradle:

```groovy
// android/build.gradle
buildscript {
    dependencies {
        classpath 'com.google.gms:google-services:4.4.0'
    }
}

// android/app/build.gradle
apply plugin: 'com.google.gms.google-services'
```

## Request Permission

```typescript
import notifee, { AuthorizationStatus } from '@notifee/react-native';

const requestPermission = async () => {
  const settings = await notifee.requestPermission();

  if (settings.authorizationStatus === AuthorizationStatus.AUTHORIZED) {
    console.log('Notification permission granted');
  } else if (settings.authorizationStatus === AuthorizationStatus.PROVISIONAL) {
    console.log('Provisional notification permission granted');
  }
};
```

## Display Notification

```typescript
import notifee, { AndroidImportance } from '@notifee/react-native';

const displayNotification = async (title: string, body: string) => {
  // Create channel (Android)
  const channelId = await notifee.createChannel({
    id: 'default',
    name: 'Default Channel',
    importance: AndroidImportance.HIGH,
  });

  await notifee.displayNotification({
    title,
    body,
    android: { channelId },
    ios: { sound: 'default' },
  });
};
```

## Handle Notification Tap

```typescript
import notifee, { EventType } from '@notifee/react-native';

// Foreground events
notifee.onForegroundEvent(({ type, detail }) => {
  if (type === EventType.PRESS) {
    // Navigate based on notification data
    const { data } = detail.notification;
    if (data?.projectId) {
      navigation.navigate('ProjectDetail', { projectId: data.projectId });
    }
  }
});

// Background events
notifee.onBackgroundEvent(async ({ type, detail }) => {
  if (type === EventType.PRESS) {
    // Handle background tap
  }
});
```

## Integration with Centrifugo

```typescript
// When receiving WebSocket message, show local notification if app is background
AppState.addEventListener('change', (state) => {
  if (state === 'background') {
    // WebSocket messages → local notifications
    centrifuge.on('message', (msg) => {
      displayNotification(msg.title, msg.body);
    });
  }
});
```
