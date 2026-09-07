# React Native Performance Best Practices

Based on Callstack's comprehensive React Native optimization guide - the gold standard from the company that maintains React Native.

## When to Use

- App feels sluggish or drops below 60fps
- Bundle size is growing beyond budget (>2MB JS, >40MB total)
- Cold start / TTI exceeds 2 seconds
- Memory usage climbs or leaks detected
- Animations stutter or skip frames
- Lists scroll with jank

## Problem → Skill Routing

| Problem | Start Here |
|---------|------------|
| FPS drops during scroll | `references/flatlist-optimization.md` → `references/reanimated-patterns.md` |
| Slow app startup | `references/tti-optimization.md` → `references/bundle-analysis.md` |
| Large bundle size | `references/barrel-exports.md` → `references/tree-shaking.md` → `references/code-splitting.md` |
| Memory warnings | `references/memory-patterns.md` → `references/memory-leaks.md` |
| JS thread blocked | `references/fps-measurement.md` → `references/react-profiling.md` |
| Animation jank | `references/reanimated-patterns.md` |
| Re-renders cascade | `references/atomic-state.md` → `references/react-compiler.md` |
| Native module slow | `references/turbo-modules.md` → `references/threading.md` |

## Performance Budgets

| Metric | Budget | Measure With |
|--------|--------|-------------|
| Cold Start | <1.5s | Flipper startup profiler |
| TTI | <2s | Custom performance marks |
| FPS | 60fps (16.6ms/frame) | Perf Monitor overlay |
| JS Bundle | <2MB | `npx react-native-bundle-visualizer` |
| Total App Size | <40MB | Xcode/Android Studio archives |
| Memory (idle) | <120MB | Flipper memory profiler |
| Memory (active) | <200MB | Flipper memory profiler |

## Reference Files

### JS Domain (9 skills)
- `references/flatlist-optimization.md` - FlatList/FlashList performance patterns
- `references/fps-measurement.md` - Measuring and diagnosing FPS drops
- `references/react-profiling.md` - React DevTools profiler workflow
- `references/memory-leaks.md` - Finding and fixing JS memory leaks
- `references/atomic-state.md` - Granular state to minimize re-renders
- `references/concurrent-react.md` - Transitions, Suspense, useDeferredValue
- `references/react-compiler.md` - React Compiler auto-memoization
- `references/reanimated-patterns.md` - UI-thread animations with Reanimated
- `references/uncontrolled-components.md` - Reducing re-renders with refs

### Native Domain (10 skills)
- `references/turbo-modules.md` - New Architecture Turbo Modules
- `references/sdks-vs-polyfills.md` - Native SDKs vs JS polyfills
- `references/tti-optimization.md` - Time to Interactive optimization
- `references/threading.md` - JS/UI/Native thread management
- `references/native-profiling.md` - Xcode Instruments & Android Studio profiling
- `references/platform-setup.md` - iOS/Android build optimization
- `references/view-flattening.md` - Reducing view hierarchy depth
- `references/memory-patterns.md` - Native memory management patterns
- `references/android-16kb.md` - Android 16KB page alignment
- `references/fabric-rendering.md` - Fabric renderer patterns

### Bundle Domain (9 skills)
- `references/barrel-exports.md` - Eliminating barrel file re-exports
- `references/js-bundle-analysis.md` - Analyzing JS bundle composition
- `references/tree-shaking.md` - Ensuring dead code elimination
- `references/app-size-analysis.md` - Total app size breakdown
- `references/r8-android.md` - R8/ProGuard Android optimization
- `references/hermes-mmap.md` - Hermes bytecode and memory mapping
- `references/native-assets.md` - Image/font/resource optimization
- `references/library-size.md` - Evaluating dependency size impact
- `references/code-splitting.md` - Lazy loading and code splitting

## Priority Order for Optimization

1. **FPS & Scrolling** (user-facing, immediately noticeable)
2. **Bundle Size** (affects download, startup, memory)
3. **TTI / Cold Start** (first impression)
4. **Native Performance** (platform-specific bottlenecks)
5. **Memory** (affects stability, background kill rate)
6. **Animations** (polish, perceived quality)
