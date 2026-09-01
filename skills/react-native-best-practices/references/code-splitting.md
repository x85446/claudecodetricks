# Code Splitting & Lazy Loading

## Screen-Level Code Splitting

```typescript
import React, { Suspense } from 'react';

// Lazy load screens (loaded on navigation)
const HomeScreen = React.lazy(() => import('./screens/HomeScreen'));
const ProfileScreen = React.lazy(() => import('./screens/ProfileScreen'));
const SettingsScreen = React.lazy(() => import('./screens/SettingsScreen'));

// In navigator
const Stack = createNativeStackNavigator();

const App = () => (
  <NavigationContainer>
    <Stack.Navigator>
      <Stack.Screen name="Home">
        {() => (
          <Suspense fallback={<ScreenSkeleton />}>
            <HomeScreen />
          </Suspense>
        )}
      </Stack.Screen>
      <Stack.Screen name="Profile">
        {() => (
          <Suspense fallback={<ScreenSkeleton />}>
            <ProfileScreen />
          </Suspense>
        )}
      </Stack.Screen>
    </Stack.Navigator>
  </NavigationContainer>
);
```

## Component-Level Lazy Loading

```typescript
// Heavy components loaded on demand
const HeavyChart = React.lazy(() => import('./components/HeavyChart'));
const RichTextEditor = React.lazy(() => import('./components/RichTextEditor'));

const AnalyticsTab = () => (
  <Suspense fallback={<ChartSkeleton />}>
    <HeavyChart data={chartData} />
  </Suspense>
);
```

## Inline Requires (Metro)

Metro's inline requires defer module loading until first use:

```javascript
// metro.config.js
module.exports = {
  transformer: {
    getTransformOptions: async () => ({
      transform: {
        inlineRequires: true,
      },
    }),
  },
};
```

Before (eager):
```javascript
const HeavyModule = require('./HeavyModule'); // Loaded at import time
```

After (inline):
```javascript
// require('./HeavyModule') is inserted at each usage point
// Module loaded only when actually used
```

## Conditional Loading

```typescript
// Load features only when enabled
const loadChat = async () => {
  const { ChatModule } = await import('./modules/chat');
  return ChatModule;
};

// Load platform-specific code
const PlatformCamera = Platform.select({
  ios: () => require('./Camera.ios'),
  android: () => require('./Camera.android'),
})!;
```

## What NOT to Code Split

- Navigation framework (always needed)
- Auth module (needed at startup)
- Theme/design system (needed everywhere)
- Small utilities (<5KB)

## What TO Code Split

- Analytics dashboards
- Rich text editors
- Chart/graph libraries
- Settings screens (rarely visited)
- Admin/debug screens
- Heavy form builders
