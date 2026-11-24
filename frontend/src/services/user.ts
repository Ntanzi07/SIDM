import { useSyncExternalStore } from 'react'

// Quem está logado. A sessão é um cookie HttpOnly (o JavaScript não lê): o
// app pergunta ao servidor (/auth/me) ao abrir e guarda a conta aqui.
// undefined = ainda perguntando; null = ninguém logado.

export type Account = {
  username: string // o nome no histórico das imagens (sai do e-mail no cadastro)
  email?: string
  name: string
  role: 'admin' | 'user'
  status: 'pending' | 'active' | 'disabled'
  created_at: string
  approved_by?: string
  approved_at?: string
  last_login?: string
}

let current: Account | null | undefined
const listeners = new Set<() => void>()

export function getAccount(): Account | null | undefined {
  return current
}

// O nome de login (o mesmo do histórico das imagens e da fila).
export function getUser(): string {
  return current?.username ?? ''
}

export function isAdmin(): boolean {
  return current?.role === 'admin'
}

export function setAccount(account: Account | null): void {
  current = account
  listeners.forEach((listener) => listener())
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

// A conta logada, atualizando a tela quando muda (entrar, sair, sessão vencida).
export function useAccount(): Account | null | undefined {
  return useSyncExternalStore(subscribe, getAccount)
}

// Só admin (apagar, Configurar, contas): para esconder o que não pode.
export function useIsAdmin(): boolean {
  return useAccount()?.role === 'admin'
}

// Quantas contas esperam aprovação (o selo do admin no menu). Quem muda a
// lista (a tela de Usuários) avisa por aqui, e o selo acompanha na hora.
let pending = 0
const pendingListeners = new Set<() => void>()

export function setPendingCount(count: number): void {
  if (count === pending) return
  pending = count
  pendingListeners.forEach((listener) => listener())
}

function subscribePending(listener: () => void): () => void {
  pendingListeners.add(listener)
  return () => { pendingListeners.delete(listener) }
}

export function usePendingCount(): number {
  return useSyncExternalStore(subscribePending, () => pending)
}
