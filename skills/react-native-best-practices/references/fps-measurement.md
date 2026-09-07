# FPS Measurement & Diagnosis

## Quick Start

1. Open Dev Menu → "Show Perf Monitor"
2. Watch JS and UI thread FPS during interactions
3. Both should stay at 60fps

## Thread Model

| Thread | Responsibility | FPS Drop Cause |
|--------|---------------|----------------|
| **JS Thread** | React renders, state, callbacks | Heavy computations, re-renders |
| **UI Thread** | Native view updates, gestures | Complex layouts, overdraw |
| **Shadow Thread** | Yoga layout calculations | Deep view hierarchies |

## Diagnosing JS Thread Drops

```typescript
// Add performance marks to suspect code
performance.mark('expensive-start');
// ... suspect code ...
performance.mark('expensive-end');
performance.measure('expensive-operation', 'expensive-start', 'expensive-end');
```

### Common JS Thread Bottlenecks
- Large state updates in scroll handlers
- JSON.parse/stringify on large objects
- Complex filtering/sorting in render
- Unoptimized re-renders (missing memo/useCallback)

## Diagnosing UI Thread Drops

### Common UI Thread Bottlenecks
- Deep view nesting (>10 levels)
- Opacity animations without native driver
- Shadow/elevation on many views simultaneously
- Large images without caching

## Tools

### React Native Performance Monitor
- Built-in: Dev Menu → "Show Perf Monitor"
- Shows: JS FPS, UI FPS, RAM usage

### Flipper
- Performance plugin for frame-by-frame analysis
- Network inspector for API latency
- Layout inspector for view hierarchy depth

### Xcode Instruments (iOS)
- Time Profiler: CPU hotspots
- Core Animation: GPU rendering
- Allocations: Memory allocation patterns

### Android Studio Profiler
- CPU Profiler: Method tracing
- GPU Rendering: Frame render times
- Memory Profiler: Heap analysis

## Benchmarks

| Scenario | Target | Warning | Critical |
|----------|--------|---------|----------|
| Idle | 60fps | <55fps | <45fps |
| Scrolling | 60fps | <45fps | <30fps |
| Animation | 60fps | <50fps | <40fps |
| Navigation | 60fps | <40fps | <30fps |
