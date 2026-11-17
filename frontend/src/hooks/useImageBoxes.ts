import { useEffect, useState } from 'react'
import { getImageBoxes, type BoxSet } from '../services/api'

// As caixas da imagem aberta, uma entrada por tarefa (vazio: sem caixas, ou
// ainda carregando).
export function useImageBoxes(sha1: string | undefined): BoxSet[] {
  const [loaded, setLoaded] = useState<{ sha1?: string; sets: BoxSet[] }>({ sets: [] })
  useEffect(() => {
    if (!sha1) return
    let active = true
    getImageBoxes(sha1)
      .then((sets) => { if (active) setLoaded({ sha1, sets }) })
      .catch(() => { if (active) setLoaded({ sha1, sets: [] }) })
    return () => { active = false }
  }, [sha1])
  return loaded.sha1 === sha1 ? loaded.sets : []
}

// "placa-letras: 7 caixas · capacete: sem objetos"
export function describeBoxSets(sets: BoxSet[]): string {
  return sets
    .map((set) => `${set.task}: ${set.boxes.length > 0 ? `${set.boxes.length} caixa(s)` : 'sem objetos'}`)
    .join(' · ')
}
