# Native Profiling Tools

## iOS - Xcode Instruments

### Time Profiler
1. Product → Profile (Cmd+I)
2. Select "Time Profiler"
3. Record during interaction
4. Look for:
   - Methods taking >16ms (frame budget)
   - Repeated allocations in hot paths
   - Main thread blocking calls

### Core Animation (GPU)
1. Select "Core Animation" instrument
2. Enable "Color Blended Layers" (red = overdraw)
3. Enable "Color Offscreen-Rendered" (yellow = expensive)
4. Target: Minimize red/yellow areas

### Allocations
1. Select "Allocations" instrument
2. Mark heap before interaction
3. Perform interaction
4. Mark heap after
5. Compare: look for growth patterns

### Common iOS Issues
- `CALayer` shadow without `shadowPath` - very expensive
- Offscreen rendering from `cornerRadius` + `clipsToBounds`
- Image decoding on main thread

## Android - Android Studio Profiler

### CPU Profiler
1. Run app in debug mode
2. Android Studio → Profiler → CPU
3. Record method trace or sample trace
4. Look for:
   - Methods on main thread >16ms
   - Blocking I/O calls
   - Excessive GC pauses

### GPU Rendering
```bash
# Enable GPU profiling bars
adb shell setprop debug.hwui.profile true
# Green line = 16ms target
# Bars above green = dropped frames
```

### Memory Profiler
1. Profiler → Memory
2. Force GC, take heap dump
3. Navigate, interact
4. Force GC, take heap dump
5. Compare retained objects

### Systrace
```bash
# Comprehensive system trace
npx react-native profile-hermes
# Opens Chrome trace viewer
```

## Quick Checks

```bash
# Android: Check for dropped frames
adb shell dumpsys gfxinfo <package_name>

# Android: Memory stats
adb shell dumpsys meminfo <package_name>

# iOS: Check GPU usage
# Xcode → Debug Navigator → GPU
```

## When to Profile Native vs JS

| Symptom | Profile | Tool |
|---------|---------|------|
| Scroll jank | JS first, then Native | Perf Monitor → Instruments/Profiler |
| Slow startup | Both | TTI marks + Time Profiler |
| Memory growth | Both | Flipper + Instruments/Profiler |
| Animation stutter | Native first | Core Animation / GPU Rendering |
| Tap response delay | JS first | React Profiler |
