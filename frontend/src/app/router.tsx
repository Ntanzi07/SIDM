import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { MainLayout } from './layouts/MainLayout'
import { ExportPage } from '../pages/Export/ExportPage'
import { PackagesPage } from '../pages/Packages/PackagesPage'
import { CreatePackagePage } from '../pages/Packages/CreatePackagePage'
import { PackageDetailPage } from '../pages/Packages/PackageDetailPage'
import { AnnotationPage } from '../pages/Annotation/AnnotationPage'
import { SettingsPage } from '../pages/Settings/SettingsPage'
import { ImageViewerPage } from '../pages/Viewer/ImageViewerPage'
import { TasksPage } from '../pages/Tasks/TasksPage'
import { ProcessPage } from '../pages/Process/ProcessPage'
import { DashboardPage } from '../pages/Dashboard/DashboardPage'
import { DatasetPage } from '../pages/Versions/DatasetPage'
import { LoginPage } from '../pages/Login/LoginPage'
import { RequireAdmin, RequireLogin } from './auth/RequireLogin'

export function Router() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<RequireLogin />}>
        <Route element={<MainLayout />}>
          <Route path="/" element={<PackagesPage />} />
          <Route path="/pacotes" element={<PackagesPage />} />
          <Route path="/pacotes/criar" element={<CreatePackagePage />} />
          {/* só admin: o Processar (o Configurar abre para todos, ver abaixo) */}
          <Route element={<RequireAdmin />}>
            <Route path="/processar" element={<ProcessPage />} />
          </Route>
          <Route path="/pacotes/:id" element={<PackageDetailPage />} />
          <Route path="/pacotes/:id/anotacao" element={<AnnotationPage />} />
          <Route path="/pacotes/:id/verificadas" element={<ImageViewerPage />} />
          {/* o Configurar abre para todos (a própria conta); o resto dele é de admin */}
          <Route path="/configurar" element={<SettingsPage />} />
          <Route path="/atalhos" element={<Navigate replace to="/configurar?aba=atalhos" />} />
          <Route path="/usuarios" element={<Navigate replace to="/configurar?aba=usuarios" />} />
          <Route path="/tarefas" element={<TasksPage />} />
          <Route path="/tarefas/:name" element={<TasksPage />} />
          <Route path="/exportar" element={<ExportPage />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/dashboard/:profile" element={<DashboardPage />} />
          {/* nomes antigos: Pesquisa e Versões viraram o Dashboard */}
          <Route path="/pesquisa" element={<Navigate replace to="/dashboard" />} />
          <Route path="/versoes" element={<Navigate replace to="/dashboard" />} />
          <Route path="/versoes/:name" element={<DatasetPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
