# Atomic State Management

## Principle

Split state into the smallest possible atoms. Components subscribe only to the state they need, minimizing re-renders.

## Zustand Atomic Pattern

```typescript
// WRONG - Monolithic store, every subscriber re-renders on any change
const useStore = create((set) => ({
  user: null,
  projects: [],
  notifications: [],
  theme: 'light',
  setUser: (user) => set({ user }),
  setProjects: (projects) => set({ projects }),
}));

// Every component using useStore() re-renders when ANY field changes
const user = useStore(state => state.user); // Still re-renders on projects change!
```

```typescript
// CORRECT - Selective subscriptions with Zustand
const useStore = create((set) => ({
  user: null,
  projects: [],
  notifications: [],
  theme: 'light',
  setUser: (user) => set({ user }),
  setProjects: (projects) => set({ projects }),
}));

// Use selectors - only re-renders when selected value changes
const user = useStore(state => state.user);
const projectCount = useStore(state => state.projects.length);
```

## Zustand Best Practices for RN

```typescript
// 1. Use shallow comparison for object selectors
import { shallow } from 'zustand/shallow';

const { user, theme } = useStore(
  state => ({ user: state.user, theme: state.theme }),
  shallow
);

// 2. Separate stores by domain
const useAuthStore = create(() => ({ user: null, token: null }));
const useProjectStore = create(() => ({ projects: [], filters: {} }));
const useUIStore = create(() => ({ theme: 'light', bottomSheet: null }));

// 3. Persist with MMKV (not AsyncStorage)
import { createJSONStorage, persist } from 'zustand/middleware';
import { MMKV } from 'react-native-mmkv';

const storage = new MMKV();
const mmkvStorage = {
  getItem: (name: string) => storage.getString(name) ?? null,
  setItem: (name: string, value: string) => storage.set(name, value),
  removeItem: (name: string) => storage.delete(name),
};

const useAuthStore = create(
  persist(
    (set) => ({ token: null }),
    { name: 'auth-storage', storage: createJSONStorage(() => mmkvStorage) }
  )
);
```

## State Location Decision Tree

| State Type | Where | Why |
|-----------|-------|-----|
| Form input value | `useState` local | Only this component needs it |
| Current screen's data | React Query | Server state, cached |
| Auth token | Zustand + MMKV | Global, persisted |
| Theme/locale | Zustand | Global, rarely changes |
| Bottom sheet open | `useState` local | Ephemeral UI state |
| WebSocket connection | Zustand | Shared across screens |
| Navigation params | React Navigation | Route-scoped |

## Anti-Patterns

- **Context for frequently-changing state** - Causes subtree re-renders
- **Redux for small apps** - Zustand is simpler, less boilerplate
- **AsyncStorage for auth tokens** - Use MMKV (10x faster) or Keychain
- **Storing derived data** - Compute with `useMemo` instead
