import { useSyncExternalStore } from 'react'
import { listTasks } from '../services/api'

// As cores escolhidas para as labels, por tarefa (Tarefas → a bolinha de cada
// label). Carregadas uma vez e guardadas aqui; quem desenha caixas usa
// useTaskColors() para redesenhar quando elas chegam ou mudam.

let colors: Record<string, Record<string, string>> = {}
let version = 0
let loading: Promise<void> | null = null
const listeners = new Set<() => void>()

function emit() {
  version += 1
  for (const listener of listeners) listener()
}

export function loadTaskColors(force = false): Promise<void> {
  if (loading && !force) return loading
  loading = listTasks()
    .then((list) => {
      colors = Object.fromEntries(list.map((task) => [task.name, task.colors ?? {}]))
      emit()
    })
    .catch(() => { /* sem as cores: ficam as automáticas */ })
  return loading
}

// Depois de salvar uma tarefa: a tela de anotação já vê a cor nova.
export function setTaskColors(task: string, taskColors: Record<string, string> | undefined) {
  colors = { ...colors, [task]: taskColors ?? {} }
  emit()
}

export function taskColor(task: string | undefined, label: string): string | undefined {
  return task ? colors[task]?.[label] : undefined
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  void loadTaskColors()
  return () => { listeners.delete(listener) }
}

// Redesenha quem chama quando as cores carregam ou mudam.
export function useTaskColors(): number {
  return useSyncExternalStore(subscribe, () => version)
}
