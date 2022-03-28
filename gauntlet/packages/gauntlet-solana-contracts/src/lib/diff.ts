import { logger } from '@chainlink/gauntlet-core/dist/utils'

enum DIFF_PROPERTY_COLOR {
  ADDED = 'green',
  REMOVED = 'red',
  NO_CHANGE = 'reset',
}

type DIFF_OPTIONS = {
  initialIndent?: string
  propertyName?: string
}

type BasicType = string | number | boolean

abstract class DiffItem {
  abstract isArray(): boolean
  abstract isFurtherComparableObject(): boolean
  abstract getData(): any
  abstract isEqual(item: any): boolean
  abstract toString(): string

  printAdded(prependStr: string = '') {
    const str = this.toString()
    logger.log(prependStr, logger.style(str, DIFF_PROPERTY_COLOR.ADDED))
  }

  printRemoved(prependStr: string = '') {
    const str = this.toString()
    logger.log(prependStr, logger.style(str, DIFF_PROPERTY_COLOR.REMOVED))
  }

  printNoChange(prependStr: string = '') {
    const str = this.toString()
    logger.log(prependStr, logger.style(str, DIFF_PROPERTY_COLOR.NO_CHANGE))
  }

  static printString(str: string) {
    logger.log(str)
  }

  static create(base: any): DiffItem {
    if (Array.isArray(base)) {
      // assume all elements are homogeneous
      const elem = base[0]

      if (Array.isArray(elem)) {
        throw new Error('Diff: nested arrays are not supported!')
      }

      if (Buffer.isBuffer(elem)) {
        return new DiffBufferArray(base)
      }

      if (typeof elem === 'object') {
        return new DiffObjectArray(base)
      }

      return new DiffBasicArray(base)
    }

    if (Buffer.isBuffer(base)) {
      return new DiffBuffer(base)
    }

    if (typeof base === 'object') {
      return new DiffObject(base)
    }

    return new DiffBasic(base)
  }
}

abstract class DiffItemPlain<T> extends DiffItem {
  item: T

  constructor(item: T) {
    super()
    this.item = item
  }

  getData(): T {
    return this.item
  }

  isArray(): boolean {
    return false
  }
}

class DiffBuffer extends DiffItemPlain<Buffer> {
  constructor(item: Buffer) {
    super(item)
  }

  toString(): string {
    return this.item.toString('hex')
  }

  isFurtherComparableObject(): boolean {
    return false
  }

  isEqual(item: Buffer): boolean {
    return this.toString() == item.toString('hex')
  }
}

class DiffObject extends DiffItemPlain<Object> {
  constructor(item: Object) {
    super(item)
  }

  toString(): string {
    throw new Error('Diff: toString() for Object is not implemented!')
  }

  isFurtherComparableObject(): boolean {
    return true
  }

  isEqual(item: Object): boolean {
    throw new Error('Diff: isEqual() for Object is not implemented!')
  }
}

class DiffBasic extends DiffItemPlain<BasicType> {
  constructor(item: BasicType) {
    super(item)
  }

  toString(): string {
    return this.item?.toString() || '<not existing>'
  }

  isFurtherComparableObject(): boolean {
    return false
  }

  isEqual(item: BasicType): boolean {
    return this.item == item
  }
}

abstract class DiffItemArray<T> extends DiffItem {
  items: Array<T> = []

  constructor(items: Array<T>) {
    super()
    this.items = items
  }

  getData(): Array<T> {
    return this.items
  }

  isArray(): boolean {
    return true
  }

  isFurtherComparableObject(): boolean {
    return false
  }

  toString(): string {
    throw new Error('Diff: toString() for item array is not implemented!')
  }

  isEqual(item: any): boolean {
    throw new Error('Diff: isEqual() for item array is not implemented!')
  }
}

class DiffBufferArray extends DiffItemArray<Buffer> {
  constructor(items: Array<Buffer>) {
    super(items)
  }
}

class DiffObjectArray extends DiffItemArray<Object> {
  constructor(items: Array<Object>) {
    super(items)
  }
}

class DiffBasicArray extends DiffItemArray<BasicType> {
  constructor(items: Array<BasicType>) {
    super(items)
  }
}

export function printDiff(existing: Object = {}, incoming: Object = {}, options?: DIFF_OPTIONS) {
  const { initialIndent = '', propertyName = '' } = options || {}
  DiffItem.printString(`${initialIndent}${propertyName}{`)
  const indent = initialIndent + '  '

  const mergedKeys = [...new Set([...Object.keys(existing), ...Object.keys(incoming)])]
  for (const prop of mergedKeys) {
    const existingProperty = DiffItem.create(existing[prop])
    const incomingProperty = DiffItem.create(incoming[prop])

    if (incomingProperty.isArray() || existingProperty.isArray()) {
      DiffItem.printString(`${indent}${prop}: [`)
      const itemsIndent = indent + ' '
      const incomingPropertyArray = incomingProperty.getData() || []
      const existingPropertyArray = existingProperty.getData() || []

      // define iteration length
      const length = Math.max(incomingPropertyArray.length, existingPropertyArray.length)

      for (let index = 0; index < length; index++) {
        const incomingItem = DiffItem.create(incomingPropertyArray[index])
        const existingItem = DiffItem.create(existingPropertyArray[index])

        if (incomingItem.isFurtherComparableObject() || existingItem.isFurtherComparableObject()) {
          printDiff(existingItem.getData(), incomingItem.getData(), {
            initialIndent: itemsIndent,
          })
          continue
        }

        if (typeof incomingItem.getData() === 'undefined') {
          existingItem.printRemoved(itemsIndent)
          continue
        }

        if (typeof existingItem.getData() === 'undefined') {
          incomingItem.printAdded(itemsIndent)
          continue
        }

        if (incomingItem.isEqual(existingItem)) {
          incomingItem.printNoChange(itemsIndent)
          continue
        }

        existingItem.printRemoved(itemsIndent)
        incomingItem.printAdded(itemsIndent)
      }

      DiffItem.printString(`${indent}]`)
      continue
    }

    if (incomingProperty.isFurtherComparableObject() || existingProperty.isFurtherComparableObject()) {
      printDiff(existingProperty.getData(), incomingProperty.getData(), {
        initialIndent: indent,
        propertyName: `${prop}: `,
      })
      continue
    }

    if (incomingProperty.isEqual(existingProperty)) {
      incomingProperty.printNoChange(`${indent}${prop}:`)
    } else {
      existingProperty.printRemoved(`${indent}${prop}:`)
      incomingProperty.printAdded(`${indent}${prop}:`)
    }
  }

  DiffItem.printString(`${initialIndent}}`)
}
