import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { MainLayout } from './layouts/MainLayout'
import { ExportPage } from '../pages/Export/ExportPage'
import { PackagesPage } from '../pages/Packages/PackagesPage'
import { CreatePackagePage } from '../pages/Packages/CreatePackagePage'
import { PackageDetailPage } from '../pages/Packages/PackageDetailPage'
import { AnnotationPage } from '../pages/Annotation/AnnotationPage'
import { SettingsPage } from '../pages/Settings/SettingsPage'

export function Router() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<MainLayout />}>
          <Route path="/" element={<PackagesPage />} />
          <Route path="/pacotes" element={<PackagesPage />} />
          <Route path="/pacotes/criar" element={<CreatePackagePage />} />
          <Route path="/configurar" element={<SettingsPage />} />
          <Route path="/pacotes/:id" element={<PackageDetailPage />} />
          <Route path="/pacotes/:id/anotacao" element={<AnnotationPage />} />
          <Route path="/exportar" element={<ExportPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
