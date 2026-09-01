# View Flattening & Hierarchy Optimization

## Problem

Deep view hierarchies cause:
- Slower layout calculations (Yoga/Shadow thread)
- More draw calls (GPU)
- Higher memory usage
- Slower touch event propagation

## Target: <10 levels of nesting for any component

## Fabric Auto-Flattening

React Native's Fabric renderer automatically flattens views that only have layout props (no visual props like backgroundColor, border, shadow).

```typescript
// This View gets auto-flattened (only layout props)
<View style={{ flex: 1, padding: 16 }}>
  <Text>Content</Text>
</View>

// This View cannot be flattened (has visual props)
<View style={{ flex: 1, padding: 16, backgroundColor: '#fff' }}>
  <Text>Content</Text>
</View>
```

## Manual Optimization

### Remove Unnecessary Wrapper Views
```typescript
// WRONG - Unnecessary nesting
<View>
  <View style={styles.row}>
    <View style={styles.cell}>
      <Text>{item.name}</Text>
    </View>
  </View>
</View>

// CORRECT - Flattened
<View style={styles.row}>
  <Text style={styles.cellText}>{item.name}</Text>
</View>
```

### Use Fragment Instead of View
```typescript
// WRONG - View just for grouping
<View>
  <Text>Line 1</Text>
  <Text>Line 2</Text>
</View>

// CORRECT - Fragment (no native view created)
<>
  <Text>Line 1</Text>
  <Text>Line 2</Text>
</>
```

### collapsable Prop (Android)
```typescript
// Hint to Android to remove this view from native hierarchy
<View collapsable={true} style={styles.layoutOnly}>
  <ChildComponent />
</View>
```

## Measuring Hierarchy Depth

### Flipper Layout Plugin
1. Open Flipper → Layout plugin
2. Select any element
3. Count nesting depth
4. Target: <10 for any leaf element

### Android
```bash
# Dump view hierarchy
adb shell dumpsys activity top | grep -A 50 "View Hierarchy"
```

## Common Anti-Patterns

| Pattern | Problem | Fix |
|---------|---------|-----|
| View wrapping every Text | Extra native view | Style Text directly |
| Nested ScrollViews | Performance + UX issues | SectionList or single scroll |
| View for spacing | Unnecessary view | margin/padding on siblings |
| Wrapper for onPress | Extra view | Use Pressable directly |
