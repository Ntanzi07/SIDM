export type Package = {
  name: string
  createdBy: string
  updatedAt: string
  imageCount: number
  verified: number
  pending: number
  inProgress: number
  coverImage?: string
  profile: string // perfil de anotação
  tasks: string[] // tarefas do CVAT em que o pacote tem imagens
}
