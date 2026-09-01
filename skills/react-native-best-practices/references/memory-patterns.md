# Native Memory Management Patterns

## Memory Budgets

| State | Budget | Action |
|-------|--------|--------|
| Idle | <120MB | Normal |
| Active | <200MB | Normal |
| Warning | >200MB | Investigate |
| Critical | >300MB | Fix immediately |

## Image Memory

Images are the #1 memory consumer in mobile apps.

```typescript
// Calculate image memory cost:
// Memory = width × height × 4 bytes (RGBA)
// 1080×1920 image = 1080 × 1920 × 4 = ~8MB in memory!

// CRITICAL: Always specify dimensions
<FastImage
  source={{ uri: imageUrl }}
  style={{ width: 100, height: 100 }}  // Downsampled to 100×100 = ~40KB
  resizeMode={FastImage.resizeMode.cover}
/>

// Request appropriately sized images from API
const thumbnailUrl = `${baseUrl}?w=200&h=200&fit=cover`;
```

## Screen Cleanup

```typescript
import { useFocusEffect } from '@react-navigation/native';

const CameraScreen = () => {
  useFocusEffect(
    useCallback(() => {
      // Screen focused - acquire resources
      camera.current?.resume();

      return () => {
        // Screen unfocused - release resources
        camera.current?.pause();
        // Release any cached bitmaps
        // Close any open connections
      };
    }, [])
  );
};
```

## Large List Memory

```typescript
<FlatList
  // Remove off-screen items from native memory (Android)
  removeClippedSubviews={Platform.OS === 'android'}
  // Reduce window of rendered items
  windowSize={5}  // Default 21, reduce for memory
  // Limit concurrent renders
  maxToRenderPerBatch={5}
/>
```

## Cache Management

```typescript
// FastImage cache control
import FastImage from 'react-native-fast-image';

// Clear cache when memory warning
AppState.addEventListener('memoryWarning', () => {
  FastImage.clearMemoryCache();
  // Don't clear disk cache unless critical
});

// Preload critical images
FastImage.preload([
  { uri: 'https://example.com/logo.png' },
]);
```

## Detecting Memory Issues

### Warning Signs
- App increasingly sluggish over time
- iOS kills app in background frequently
- Android OOM crashes
- Images appear blank or fail to load

### Monitoring
```typescript
// Log memory periodically in dev
if (__DEV__) {
  setInterval(() => {
    const used = performance?.memory?.usedJSHeapSize;
    if (used) console.log(`JS Heap: ${(used / 1024 / 1024).toFixed(1)}MB`);
  }, 10000);
}
```
