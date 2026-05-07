export type ZipEntry = {
  path: string
  data: BlobPart
  mimeType?: string
  modifiedAt?: Date
}

function crc32(data: Uint8Array): number {
  let crc = ~0
  for (const byte of data) {
    crc ^= byte
    for (let bit = 0; bit < 8; bit += 1) {
      crc = (crc >>> 1) ^ (0xedb88320 & -(crc & 1))
    }
  }
  return ~crc >>> 0
}

function dosDateTime(date: Date): { date: number; time: number } {
  const year = Math.max(1980, date.getFullYear())
  return {
    time: (date.getHours() << 11) | (date.getMinutes() << 5) | Math.floor(date.getSeconds() / 2),
    date: ((year - 1980) << 9) | ((date.getMonth() + 1) << 5) | date.getDate(),
  }
}

function u16(value: number): number[] {
  return [value & 0xff, (value >>> 8) & 0xff]
}

function u32(value: number): number[] {
  return [value & 0xff, (value >>> 8) & 0xff, (value >>> 16) & 0xff, (value >>> 24) & 0xff]
}

function normalizeZipPath(path: string): string {
  return path
    .replace(/\\/g, '/')
    .replace(/^\/+/, '')
    .split('/')
    .filter((part) => part && part !== '.' && part !== '..')
    .join('/')
}

function blobPart(bytes: Uint8Array): BlobPart {
  const copy = new Uint8Array(bytes.length)
  copy.set(bytes)
  return copy.buffer
}

export async function createZipBlob(entries: ZipEntry[]): Promise<Blob> {
  const encoder = new TextEncoder()
  const chunks: Uint8Array[] = []
  const central: Uint8Array[] = []
  let offset = 0

  for (const entry of entries) {
    const name = normalizeZipPath(entry.path)
    if (!name) continue
    const nameBytes = encoder.encode(name)
    const payload = new Uint8Array(await new Blob([entry.data], { type: entry.mimeType }).arrayBuffer())
    const crc = crc32(payload)
    const { date, time } = dosDateTime(entry.modifiedAt ?? new Date())

    const localHeader = new Uint8Array([
      ...u32(0x04034b50),
      ...u16(20),
      ...u16(0x0800),
      ...u16(0),
      ...u16(time),
      ...u16(date),
      ...u32(crc),
      ...u32(payload.length),
      ...u32(payload.length),
      ...u16(nameBytes.length),
      ...u16(0),
      ...nameBytes,
    ])
    chunks.push(localHeader, payload)

    const centralHeader = new Uint8Array([
      ...u32(0x02014b50),
      ...u16(20),
      ...u16(20),
      ...u16(0x0800),
      ...u16(0),
      ...u16(time),
      ...u16(date),
      ...u32(crc),
      ...u32(payload.length),
      ...u32(payload.length),
      ...u16(nameBytes.length),
      ...u16(0),
      ...u16(0),
      ...u16(0),
      ...u16(0),
      ...u32(0),
      ...u32(offset),
      ...nameBytes,
    ])
    central.push(centralHeader)
    offset += localHeader.length + payload.length
  }

  const centralOffset = offset
  const centralSize = central.reduce((sum, item) => sum + item.length, 0)
  const end = new Uint8Array([
    ...u32(0x06054b50),
    ...u16(0),
    ...u16(0),
    ...u16(central.length),
    ...u16(central.length),
    ...u32(centralSize),
    ...u32(centralOffset),
    ...u16(0),
  ])

  return new Blob([...chunks.map(blobPart), ...central.map(blobPart), blobPart(end)], { type: 'application/zip' })
}
