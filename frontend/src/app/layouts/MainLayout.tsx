import { Outlet } from 'react-router-dom'
import { Header } from '../../components/common/Header/Header'
import { Toaster } from '../../components/Toast/Toast'
import { ConfirmDialog } from '../../components/ConfirmDialog/ConfirmDialog'
import { ScrollTop } from '../../components/ScrollTop/ScrollTop'

export function MainLayout() {
  return (
    <>
      <Header />
      <main>
        <Outlet />
      </main>
      <ScrollTop />
      <Toaster />
      <ConfirmDialog />
    </>
  )
}
