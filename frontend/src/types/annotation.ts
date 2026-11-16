export type Vehicle = {
  plate: string
  plate_type: string
  plate_category: string
  type: string
  color: string
}

export type ImageDetail = {
  sha1: string
  package: string
  status: 'pending' | 'verified'
  original_name: string
  file_name?: string
  ext: string
  size: number
  equipment?: string
  camera?: string
  direction?: string
  resolution?: string
  capture_angle?: string
  day_time?: string
  image_range?: string
  weather?: string
  conditions?: string[]
  captured_at?: string
  vehicles?: Vehicle[]
  claimed_by?: string
}

export type AnnotationForm = {
  equipment: string
  camera: string
  direction: string
  resolution: string
  capture_angle: string
  day_time: string
  image_range: string
  weather: string
  conditions: string[]
  captured_at: string
  vehicles: Vehicle[]
}

export type AnnotationCodes = {
  direction?: Record<string, string>
  resolution?: Record<string, string>
  capture_angle?: Record<string, string>
  day_time?: Record<string, string>
  image_range?: Record<string, string>
  weather?: Record<string, string>
  conditions?: Record<string, string>
  plate_type?: Record<string, string>
  plate_category?: Record<string, string>
  vehicle_type?: Record<string, string>
  vehicle_color?: Record<string, string>
}
