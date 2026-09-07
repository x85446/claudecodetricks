# React Compiler for React Native

## What It Does

React Compiler (formerly React Forget) automatically memoizes components and hooks, eliminating the need for manual `useMemo`, `useCallback`, and `React.memo`.

## Setup

```bash
npm install -D babel-plugin-react-compiler
```

```javascript
// babel.config.js
module.exports = {
  plugins: [
    ['babel-plugin-react-compiler', {
      // Target React Native
      target: '18', // or '19' when available
    }],
  ],
};
```

## What It Auto-Memoizes

```typescript
// BEFORE React Compiler - manual memoization needed
const MyComponent = React.memo(({ items, onPress }) => {
  const filtered = useMemo(
    () => items.filter(i => i.active),
    [items]
  );
  const handlePress = useCallback(
    (id: string) => onPress(id),
    [onPress]
  );

  return <FlatList data={filtered} renderItem={...} />;
});

// AFTER React Compiler - auto-memoized
const MyComponent = ({ items, onPress }) => {
  const filtered = items.filter(i => i.active);
  const handlePress = (id: string) => onPress(id);

  return <FlatList data={filtered} renderItem={...} />;
};
// Compiler automatically adds memoization where beneficial
```

## Rules of React (Must Follow)

The compiler requires strict adherence to React rules:

1. **No side effects during render** - No mutations, no external state reads without hooks
2. **Stable hook call order** - No conditional hooks
3. **Immutable props and state** - Never mutate, always create new objects
4. **Pure render functions** - Same inputs = same output

## Opt-Out

```typescript
// Opt out a specific component
function MyComponent() {
  'use no memo'; // Disable compiler for this component
  // ...
}
```

## Compatibility

- Works with Hermes engine
- Works with React Native 0.74+
- Compatible with Reanimated (worklets are excluded automatically)
- Metro bundler compatible via babel config
