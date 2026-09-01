# Time to Interactive (TTI) Optimization

## Target: <2s Cold Start, <1s Warm Start

## Measurement

```typescript
// Add to App.tsx or index.js
const APP_START = global.performance?.now?.() || Date.now();

const App = () => {
  useEffect(() => {
    const tti = (global.performance?.now?.() || Date.now()) - APP_START;
    console.log(`TTI: ${tti}ms`);
    // Report to analytics
  }, []);
  // ...
};
```

## Optimization Strategies (Priority Order)

### 1. Reduce JS Bundle Parse Time
- Enable Hermes (pre-compiled bytecode, no parse time)
- Verify: `hermes_enabled: true` in both platforms
- Check with: `adb shell getprop | grep hermes` (Android)

### 2. Lazy Load Screens
```typescript
// WRONG - All screens loaded at startup
import HomeScreen from './screens/HomeScreen';
import ProfileScreen from './screens/ProfileScreen';
import SettingsScreen from './screens/SettingsScreen';

// CORRECT - Screens loaded on navigation
const HomeScreen = React.lazy(() => import('./screens/HomeScreen'));
const ProfileScreen = React.lazy(() => import('./screens/ProfileScreen'));
const SettingsScreen = React.lazy(() => import('./screens/SettingsScreen'));
```

### 3. Defer Non-Critical Initialization
```typescript
const App = () => {
  useEffect(() => {
    // Critical path: auth check, navigation setup
    initAuth();

    // Defer non-critical work
    InteractionManager.runAfterInteractions(() => {
      initAnalytics();
      prefetchImages();
      setupPushNotifications();
    });
  }, []);
};
```

### 4. Minimize Startup Dependencies
- Audit `require()` calls at module level
- Move heavy imports inside functions
- Use TurboModules (lazy loaded vs bridge modules loaded eagerly)

### 5. Optimize Splash Screen Transition
```typescript
import BootSplash from 'react-native-bootsplash';

const App = () => {
  useEffect(() => {
    const init = async () => {
      await loadEssentialData();
      await BootSplash.hide({ fade: true });
    };
    init();
  }, []);
};
```

### 6. Reduce Initial Render Complexity
- First screen should be simple (skeleton → content)
- Defer heavy components with `InteractionManager`
- Use placeholder/skeleton screens

## What Slows Startup

| Cause | Impact | Fix |
|-------|--------|-----|
| Large JS bundle | +100ms per MB | Code split, tree shake |
| Eager native module init | +50-200ms each | Turbo Modules (lazy) |
| Sync storage reads | +50-100ms | MMKV (sync but fast) or async |
| Network calls before render | +200-2000ms | Show skeleton, fetch async |
| Heavy first screen | +100-500ms | Simplify, defer |
| Debug mode | +500-2000ms | Expected, test release builds |

## Release Build Testing

**Always measure TTI in release builds**, not debug:
```bash
# iOS
npx react-native run-ios --mode Release

# Android
npx react-native run-android --variant release
```
