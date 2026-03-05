import { createRouter, createWebHashHistory } from 'vue-router'
import FileView from '../views/FileView.vue'
import VaultView from '../views/VaultView.vue'

const routes = [
    {
        path: '/vault/:name',
        name: 'VaultView',
        component: VaultView,
        props: true
    },
    {
        path: '/vault/:name/file/:id',
        name: 'FileView',
        component: FileView,
        props: true
    },
    {
        path: '/vault/:name/settings',
        name: 'VaultSettings',
        component: () => import('../views/VaultSettings.vue'),
        props: true
    }
]

const router = createRouter({
    history: createWebHashHistory(),
    routes
})

export default router
