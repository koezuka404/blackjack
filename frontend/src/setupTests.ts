import '@testing-library/jest-dom'

if (typeof window !== 'undefined' && typeof window.localStorage?.getItem !== 'function') {
  const storage = new Map<string, string>()
  Object.defineProperty(window, 'localStorage', {
    value: {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => {
        storage.set(key, String(value))
      },
      removeItem: (key: string) => {
        storage.delete(key)
      },
      clear: () => {
        storage.clear()
      },
      key: (index: number) => Array.from(storage.keys())[index] ?? null,
      get length() {
        return storage.size
      },
    },
    configurable: true,
  })
}
