# React Profiling for React Native

## Setup

```bash
# React DevTools (standalone)
npx react-devtools
```

Connect to your running app via the Dev Menu → "Debug" or Flipper React DevTools plugin.

## Profiler Workflow

1. **Open Profiler tab** in React DevTools
2. **Start recording** before the interaction
3. **Perform the interaction** (scroll, navigate, tap)
4. **Stop recording**
5. **Analyze the flame chart**

## What to Look For

### Unnecessary Re-renders
- Components re-rendering without visible change
- Parent re-renders cascading to all children
- Fix: `React.memo`, `useMemo`, `useCallback`

### Expensive Renders
- Single component taking >16ms to render
- Fix: Split component, memoize expensive calculations

### Cascading Updates
- One state change triggering multiple render passes
- Fix: Batch state updates, use `useReducer` for related state

## Common Patterns

```typescript
// PROBLEM: Object identity changes every render
const MyComponent = ({ items }) => {
  const filtered = items.filter(i => i.active); // New array every render
  return <FlatList data={filtered} />;
};

// FIX: Memoize derived data
const MyComponent = ({ items }) => {
  const filtered = useMemo(
    () => items.filter(i => i.active),
    [items]
  );
  return <FlatList data={filtered} />;
};
```

```typescript
// PROBLEM: Inline callback creates new reference
<Pressable onPress={() => handlePress(item.id)} />

// FIX: Stable callback reference
const handleItemPress = useCallback((id: string) => {
  handlePress(id);
}, [handlePress]);
```

## Highlight Updates

In React DevTools Settings → "Highlight updates when components render"
- Green flash = fast render (<16ms)
- Yellow flash = moderate render
- Red flash = slow render (>16ms) - investigate immediately
