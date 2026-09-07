# Reanimated Animation Patterns

## Core Principle

All animations run on the **UI thread** via worklets. The JS thread is never blocked.

## Basic Pattern

```typescript
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withTiming,
} from 'react-native-reanimated';

const FadeInView = () => {
  const opacity = useSharedValue(0);
  const translateY = useSharedValue(20);

  const animatedStyle = useAnimatedStyle(() => ({
    opacity: opacity.value,
    transform: [{ translateY: translateY.value }],
  }));

  useEffect(() => {
    opacity.value = withTiming(1, { duration: 300 });
    translateY.value = withSpring(0);
  }, []);

  return <Animated.View style={[styles.container, animatedStyle]} />;
};
```

## Gesture-Driven Animations

```typescript
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  runOnJS,
} from 'react-native-reanimated';

const SwipeableCard = ({ onDismiss }: { onDismiss: () => void }) => {
  const translateX = useSharedValue(0);

  const gesture = Gesture.Pan()
    .onUpdate((event) => {
      translateX.value = event.translationX;
    })
    .onEnd((event) => {
      if (Math.abs(event.translationX) > SWIPE_THRESHOLD) {
        translateX.value = withTiming(
          event.translationX > 0 ? SCREEN_WIDTH : -SCREEN_WIDTH,
          {},
          () => runOnJS(onDismiss)()
        );
      } else {
        translateX.value = withSpring(0);
      }
    });

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ translateX: translateX.value }],
  }));

  return (
    <GestureDetector gesture={gesture}>
      <Animated.View style={[styles.card, animatedStyle]}>
        {/* Card content */}
      </Animated.View>
    </GestureDetector>
  );
};
```

## Layout Animations

```typescript
import Animated, { FadeIn, FadeOut, Layout } from 'react-native-reanimated';

// Entering/Exiting animations
<Animated.View entering={FadeIn.duration(300)} exiting={FadeOut}>
  <Text>Animated content</Text>
</Animated.View>

// Layout transitions (when list items reorder)
<Animated.View layout={Layout.springify()}>
  <Text>{item.title}</Text>
</Animated.View>
```

## Anti-Patterns

```typescript
// WRONG - Animating on JS thread
const [opacity, setOpacity] = useState(1);
useEffect(() => {
  const interval = setInterval(() => setOpacity(prev => prev - 0.1), 50);
  return () => clearInterval(interval);
}, []);

// WRONG - Animated API without native driver
Animated.timing(opacity, {
  toValue: 1,
  useNativeDriver: false, // Runs on JS thread!
}).start();

// CORRECT - Reanimated (UI thread)
const opacity = useSharedValue(0);
opacity.value = withTiming(1, { duration: 300 });
```

## Performance Tips

- Use `useAnimatedStyle` (not `useAnimatedProps`) for view styles
- Minimize worklet closures - pass shared values, not JS objects
- Use `cancelAnimation()` in cleanup to prevent orphaned animations
- Prefer `withSpring` over `withTiming` for natural feel
- Use `interpolate` for value mapping instead of JS math in worklets
