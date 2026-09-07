# JS/UI/Native Thread Management

## Thread Model

React Native runs on three main threads:

| Thread | Purpose | Blocks UI If Overloaded |
|--------|---------|------------------------|
| **JS Thread** | React renders, business logic, state | Yes (indirectly) |
| **UI Thread** | Native view updates, touch events | Yes (directly - ANR/freeze) |
| **Shadow Thread** | Yoga layout calculations | Yes (delayed layout) |

New Architecture adds:
| Thread | Purpose |
|--------|---------|
| **Background Thread** | Turbo Module async operations |

## Keeping JS Thread Free

```typescript
// WRONG - Heavy computation on JS thread during render
const ExpensiveComponent = ({ data }) => {
  const processed = data.map(item => heavyTransform(item)); // Blocks JS thread
  return <FlatList data={processed} />;
};

// CORRECT - Offload to InteractionManager
const ExpensiveComponent = ({ data }) => {
  const [processed, setProcessed] = useState([]);

  useEffect(() => {
    const task = InteractionManager.runAfterInteractions(() => {
      setProcessed(data.map(item => heavyTransform(item)));
    });
    return () => task.cancel();
  }, [data]);

  return <FlatList data={processed} />;
};
```

## Keeping UI Thread Free

```typescript
// WRONG - JS-driven animation blocks UI on bridge calls
Animated.timing(value, {
  toValue: 1,
  useNativeDriver: false, // Runs through bridge
}).start();

// CORRECT - Native driver, UI thread only
Animated.timing(value, {
  toValue: 1,
  useNativeDriver: true, // Runs on UI thread directly
}).start();

// BEST - Reanimated worklets, zero bridge
const style = useAnimatedStyle(() => ({
  opacity: withTiming(1),
}));
```

## Background Processing

```typescript
// For heavy tasks, use native background threads
import { NativeModules } from 'react-native';

// Or use InteractionManager for JS-thread deferral
InteractionManager.runAfterInteractions(async () => {
  await processLargeDataset();
  await syncOfflineChanges();
});
```

## Common Thread Violations

| Problem | Thread | Symptom | Fix |
|---------|--------|---------|-----|
| Heavy sort/filter in render | JS | Dropped frames | useMemo + InteractionManager |
| JSON.stringify large objects | JS | UI freeze | Chunk or offload |
| Synchronous storage read | JS | Startup delay | MMKV or async |
| Layout animation without native | UI | Jank | Reanimated |
| Many simultaneous shadows | UI | Slow scroll | Reduce shadows, use elevation |
| Deep view nesting | Shadow | Slow layout | Flatten hierarchy |
