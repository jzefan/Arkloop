// jsdom 未实现 Blob URL；ArtifactIframe 等依赖此方法。
if (typeof URL.createObjectURL !== 'function') {
  Object.defineProperty(URL, 'createObjectURL', {
    configurable: true,
    writable: true,
    value: (_blob: Blob) => 'blob:jsdom-polyfill',
  })
}
if (typeof URL.revokeObjectURL !== 'function') {
  Object.defineProperty(URL, 'revokeObjectURL', {
    configurable: true,
    writable: true,
    value: (_url: string) => {},
  })
}
