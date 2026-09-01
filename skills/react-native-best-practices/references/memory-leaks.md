# Finding & Fixing Memory Leaks

## Common Leak Sources in React Native

### 1. Event Listeners Not Cleaned Up
```typescript
// LEAK
useEffect(() => {
  const subscription = AppState.addEventListener('change', handler);
  // Missing cleanup!
}, []);

// FIXED
useEffect(() => {
  const subscription = AppState.addEventListener('change', handler);
  return () => subscription.remove();
}, []);
```

### 2. Timers Not Cleared
```typescript
// LEAK
useEffect(() => {
  const interval = setInterval(poll, 5000);
  // Missing cleanup!
}, []);

// FIXED
useEffect(() => {
  const interval = setInterval(poll, 5000);
  return () => clearInterval(interval);
}, []);
```

### 3. State Updates After Unmount
```typescript
// LEAK - setState on unmounted component
useEffect(() => {
  fetchData().then(data => setData(data));
}, []);

// FIXED - AbortController
useEffect(() => {
  const controller = new AbortController();
  fetchData({ signal: controller.signal })
    .then(data => setData(data))
    .catch(err => {
      if (err.name !== 'AbortError') throw err;
    });
  return () => controller.abort();
}, []);
```

### 4. Navigation Listeners
```typescript
// FIXED - useFocusEffect auto-cleans up
useFocusEffect(
  useCallback(() => {
    const unsubscribe = subscribeToUpdates();
    return () => unsubscribe();
  }, [])
);
```

### 5. Animated Values
```typescript
// LEAK - Animation keeps running
Animated.loop(animation).start();

// FIXED
useEffect(() => {
  const anim = Animated.loop(animation);
  anim.start();
  return () => anim.stop();
}, []);
```

### 6. WebSocket/Centrifugo Connections
```typescript
// FIXED
useEffect(() => {
  const ws = new WebSocket(url);
  return () => ws.close();
}, []);
```

## Detection

### Flipper Memory Profiler
1. Open Flipper → Memory plugin
2. Take heap snapshot before interaction
3. Perform interaction (navigate back and forth)
4. Take heap snapshot after
5. Compare: retained objects should not grow

### Warning Signs
- Memory increases monotonically during use
- App gets slower over time
- iOS kills app in background frequently
- Android "Application Not Responding" (ANR)

## Screen-Level Cleanup Pattern

```typescript
const MyScreen = () => {
  // Group all subscriptions for clean teardown
  useEffect(() => {
    const subs: (() => void)[] = [];

    subs.push(EventEmitter.addListener('event', handler).remove);
    subs.push(AppState.addEventListener('change', appHandler).remove);

    return () => subs.forEach(unsub => unsub());
  }, []);
};
```
