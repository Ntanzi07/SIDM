// Os tipos da página Processar (espelham o módulo processing do backend).

export type ProcessingScope = 'pending' | 'verified' | 'all'

export type ProcessingStatus = 'running' | 'done' | 'failed' | 'canceled' | 'interrupted'

// Um modelo de IA ligado no servidor, com o estado de agora.
export type ProcessingModel = {
  key: string
  label: string
  description: string
  default_task: string // a tarefa onde as caixas vão, se não escolher outra
  labels: string[]
  default_min_score: number // o score mínimo que a tela sugere
  score_help: string // a que o score mínimo se aplica
  available: boolean
  error?: string
  busy: boolean // há um processamento rodando nele
}

export type ProcessingRunInput = {
  model: string
  packages: string[]
  scope: ProcessingScope
  task?: string
  min_score?: number
}

// Um processamento: o pedido, o progresso e o relatório.
export type ProcessingRun = {
  id: string
  model: string
  packages: string[]
  scope: ProcessingScope
  task: string
  min_score: number
  status: ProcessingStatus
  message?: string
  total: number
  done: number
  written: number // imagens com caixas gravadas
  boxes: number
  skipped: number // puladas: as caixas vieram do CVAT ou foram editadas
  skipped_ids?: string[]
  error_count: number
  errors?: { sha1: string; msg: string }[]
  started_by: string
  started_at: string
  finished_at?: string
}
