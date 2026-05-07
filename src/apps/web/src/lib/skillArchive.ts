function slugFromFilename(name: string): string {
  return name
    .replace(/\.[^.]+$/, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    || 'imported-skill'
}

const crcTable = (() => {
  const table = new Uint32Array(256)
  for (let i = 0; i < 256; i += 1) {
    let c = i
    for (let k = 0; k < 8; k += 1) {
      c = (c & 1) ? (0xedb88320 ^ (c >>> 1)) : (c >>> 1)
    }
    table[i] = c >>> 0
  }
  return table
})()

function crc32(bytes: Uint8Array): number {
  let crc = 0xffffffff
  for (const byte of bytes) {
    crc = crcTable[(crc ^ byte) & 0xff] ^ (crc >>> 8)
  }
  return (crc ^ 0xffffffff) >>> 0
}

function writeU16(out: number[], value: number) {
  out.push(value & 0xff, (value >>> 8) & 0xff)
}

function writeU32(out: number[], value: number) {
  out.push(value & 0xff, (value >>> 8) & 0xff, (value >>> 16) & 0xff, (value >>> 24) & 0xff)
}

function textBytes(text: string): Uint8Array {
  return new TextEncoder().encode(text)
}

export async function markdownSkillToZip(file: File): Promise<File> {
  const skillKey = slugFromFilename(file.name)
  const markdown = await file.text()
  const files = [
    {
      name: 'skill.yaml',
      bytes: textBytes(`skill_key: ${skillKey}\nversion: "1.0.0"\ndisplay_name: ${skillKey}\ninstruction_path: SKILL.md\n`),
    },
    { name: 'SKILL.md', bytes: textBytes(markdown) },
  ]
  const out: number[] = []
  const central: number[] = []
  for (const item of files) {
    const name = textBytes(item.name)
    const crc = crc32(item.bytes)
    const offset = out.length
    writeU32(out, 0x04034b50)
    writeU16(out, 20)
    writeU16(out, 0)
    writeU16(out, 0)
    writeU16(out, 0)
    writeU16(out, 0)
    writeU32(out, crc)
    writeU32(out, item.bytes.length)
    writeU32(out, item.bytes.length)
    writeU16(out, name.length)
    writeU16(out, 0)
    out.push(...name, ...item.bytes)

    writeU32(central, 0x02014b50)
    writeU16(central, 20)
    writeU16(central, 20)
    writeU16(central, 0)
    writeU16(central, 0)
    writeU16(central, 0)
    writeU16(central, 0)
    writeU32(central, crc)
    writeU32(central, item.bytes.length)
    writeU32(central, item.bytes.length)
    writeU16(central, name.length)
    writeU16(central, 0)
    writeU16(central, 0)
    writeU16(central, 0)
    writeU16(central, 0)
    writeU32(central, 0)
    writeU32(central, offset)
    central.push(...name)
  }
  const centralOffset = out.length
  out.push(...central)
  writeU32(out, 0x06054b50)
  writeU16(out, 0)
  writeU16(out, 0)
  writeU16(out, files.length)
  writeU16(out, files.length)
  writeU32(out, central.length)
  writeU32(out, centralOffset)
  writeU16(out, 0)
  return new File([new Uint8Array(out)], `${skillKey}.zip`, { type: 'application/zip' })
}
