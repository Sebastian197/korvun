const classMarker = 'class WebpackBarPlugin extends '
const exportMarkers = [
  '\nmodule.exports = WebpackBarPlugin;',
  '\nexport { WebpackBarPlugin as default };',
]

export function patchWebpackBarSource(source) {
  const classStart = source.indexOf(classMarker)
  if (classStart === -1) {
    throw new Error('WebpackBarPlugin class was not found')
  }

  const classEnd = exportMarkers
    .map((marker) => source.indexOf(marker, classStart))
    .filter((index) => index !== -1)
    .sort((left, right) => left - right)[0]

  if (classEnd === undefined) {
    throw new Error('WebpackBarPlugin export was not found')
  }

  const pluginSource = source.slice(classStart, classEnd)
  const compatibleField = '__publicField(this, "webpackBarOptions")'

  if (pluginSource.includes(compatibleField)) {
    if (pluginSource.includes('this.options')) {
      throw new Error('WebpackBarPlugin is only partially compatible')
    }
    return source
  }

  if (!pluginSource.includes('__publicField(this, "options")')) {
    throw new Error('WebpackBarPlugin options field has an unexpected shape')
  }
  if (!pluginSource.includes('this.options = Object.assign({}, DEFAULTS, options)')) {
    throw new Error('WebpackBarPlugin constructor has an unexpected shape')
  }

  const patchedPlugin = pluginSource
    .replace('__publicField(this, "options")', compatibleField)
    .replaceAll('this.options', 'this.webpackBarOptions')

  if (patchedPlugin.includes('this.options')) {
    throw new Error('WebpackBarPlugin still overwrites ProgressPlugin options')
  }

  return source.slice(0, classStart) + patchedPlugin + source.slice(classEnd)
}
