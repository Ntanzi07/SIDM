import type { ImportProfile } from '../services/api'

const DEFAULT_ANNOTATION_PROFILE = 'veiculos'

// O perfil de anotação que um perfil de import preenche.
export function targetOf(profile: ImportProfile): string {
  return profile.annotation_profile || DEFAULT_ANNOTATION_PROFILE
}

// Os perfis de import feitos para o perfil de anotação do pacote primeiro.
// Os outros também servem, mas só preenchem o que o pacote tem (o resto vai
// para "fora do perfil" no relatório).
export function forPackage(profiles: ImportProfile[], annotationProfile: string | undefined): ImportProfile[] {
  const target = annotationProfile || DEFAULT_ANNOTATION_PROFILE
  return [...profiles].sort((a, b) => Number(targetOf(a) !== target) - Number(targetOf(b) !== target))
}

// O nome no <select>, avisando quando é de outro perfil de anotação.
export function optionLabel(profile: ImportProfile, annotationProfile: string | undefined, prefix = ''): string {
  const target = annotationProfile || DEFAULT_ANNOTATION_PROFILE
  const other = targetOf(profile) !== target ? ` (feito para ${targetOf(profile)})` : ''
  return `${prefix}${profile.name}${other}`
}
