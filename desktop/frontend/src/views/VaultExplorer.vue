<script setup lang="ts">
import {
    ArrowLeft,
    File,
    FileText,
    Folder,
    Image,
    MoreVertical,
    Music,
    Search,
    Video
} from 'lucide-vue-next'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const currentPath = computed(() => (route.params.path as string) || '')

const searchQuery = ref('')

// Mock data
const files = ref([
  { name: 'Documents', type: 'folder', size: '-', modified: '2023-10-25' },
  { name: 'Images', type: 'folder', size: '-', modified: '2023-10-24' },
  { name: 'passwords.txt', type: 'file', size: '2 KB', modified: '2023-10-23' },
  { name: 'budget_2024.pdf', type: 'file', size: '1.4 MB', modified: '2023-10-22' },
  { name: 'profile.jpg', type: 'file', size: '3.2 MB', modified: '2023-10-21' },
])

const breadcrumbs = computed(() => {
  const parts = currentPath.value.split('/').filter(Boolean)
  return [
    { name: 'Root', path: '' },
    ...parts.map((part, index) => ({
      name: part,
      path: parts.slice(0, index + 1).join('/')
    }))
  ]
})

const filteredFiles = computed(() => {
  if (!searchQuery.value) return files.value
  return files.value.filter(f => f.name.toLowerCase().includes(searchQuery.value.toLowerCase()))
})

const navigateTo = (path: string) => {
  router.push({ name: 'VaultExplorer', params: { path } })
}

const openItem = (item: any) => {
  if (item.type === 'folder') {
    const newPath = currentPath.value ? `${currentPath.value}/${item.name}` : item.name
    navigateTo(newPath)
  } else {
    console.log('Opening file:', item.name)
  }
}

const getFileIcon = (name: string, type: string) => {
  if (type === 'folder') return Folder
  const ext = name.split('.').pop()?.toLowerCase()
  if (['jpg', 'png', 'gif'].includes(ext || '')) return Image
  if (['mp4', 'mov'].includes(ext || '')) return Video
  if (['mp3', 'wav'].includes(ext || '')) return Music
  if (['txt', 'md', 'doc', 'pdf'].includes(ext || '')) return FileText
  return File
}
</script>

<template>
  <div class="explorer-container">
    <!-- Header / Toolbar -->
    <header class="explorer-header">
      <div class="left-section">
        <button @click="router.push({ name: 'VaultList' })" class="btn-icon">
          <ArrowLeft :size="20" />
        </button>
        <div class="breadcrumbs">
          <span 
            v-for="(crumb, index) in breadcrumbs" 
            :key="crumb.path" 
            class="crumb"
            :class="{ active: index === breadcrumbs.length - 1 }"
            @click="navigateTo(crumb.path)"
          >
            {{ crumb.name }}
          </span>
        </div>
      </div>
      
      <div class="right-section">
        <div class="search-box">
          <Search :size="16" class="search-icon" />
          <input v-model="searchQuery" placeholder="Search..." />
        </div>
      </div>
    </header>

    <!-- File List -->
    <div class="file-list-container">
      <table class="file-table">
        <thead>
          <tr>
            <th class="col-icon"></th>
            <th class="col-name">Name</th>
            <th class="col-size">Size</th>
            <th class="col-date">Modified</th>
            <th class="col-actions"></th>
          </tr>
        </thead>
        <tbody>
          <tr 
            v-for="file in filteredFiles" 
            :key="file.name" 
            @dblclick="openItem(file)"
            class="file-row"
          >
            <td class="col-icon">
              <component :is="getFileIcon(file.name, file.type)" :size="20" :class="file.type === 'folder' ? 'folder-icon' : 'file-icon'" />
            </td>
            <td class="col-name">{{ file.name }}</td>
            <td class="col-size">{{ file.size }}</td>
            <td class="col-date">{{ file.modified }}</td>
            <td class="col-actions">
              <button class="btn-icon-sm"><MoreVertical :size="16" /></button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="filteredFiles.length === 0" class="empty-folder">
        <p>No items found</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.explorer-container {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: var(--bg-app);
}

.explorer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.5rem;
  border-bottom: 1px solid var(--border);
  background: var(--bg-surface);
}

.left-section {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.btn-icon {
  padding: 0.5rem;
  background: transparent;
  border: none;
  color: var(--text-muted);
}

.btn-icon:hover {
  background: var(--bg-surface-hover);
  color: var(--text-main);
}

.breadcrumbs {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.9rem;
  color: var(--text-muted);
}

.crumb {
  cursor: pointer;
  padding: 0.25rem 0.5rem;
  border-radius: var(--radius);
  transition: all 0.2s;
}

.crumb:hover {
  background: var(--bg-surface-hover);
  color: var(--text-main);
}

.crumb.active {
  font-weight: 600;
  color: var(--text-main);
  cursor: default;
}

.crumb:not(:last-child)::after {
  content: '/';
  margin-left: 0.5rem;
  opacity: 0.5;
}

.right-section {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.search-box {
  position: relative;
  width: 250px;
}

.search-icon {
  position: absolute;
  left: 0.75rem;
  top: 50%;
  transform: translateY(-50%);
  color: var(--text-muted);
}

.search-box input {
  padding-left: 2.5rem;
}

.file-list-container {
  flex: 1;
  overflow-y: auto;
  padding: 0 1.5rem;
}

.file-table {
  width: 100%;
  border-collapse: collapse;
  text-align: left;
}

.file-table th {
  padding: 0.75rem 1rem;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  border-bottom: 1px solid var(--border);
  position: sticky;
  top: 0;
  background: var(--bg-app);
  z-index: 10;
}

.file-row {
  border-bottom: 1px solid var(--border);
  cursor: default;
  transition: background 0.1s;
}

.file-row:hover {
  background: var(--bg-surface);
}

.file-row td {
  padding: 0.75rem 1rem;
  font-size: 0.9rem;
  color: var(--text-main);
}

.col-icon { width: 40px; text-align: center; }
.col-name { width: 40%; font-weight: 500; }
.col-size { width: 15%; color: var(--text-muted); }
.col-date { width: 25%; color: var(--text-muted); }
.col-actions { width: 50px; text-align: right; }

.folder-icon { color: var(--primary); fill: currentColor; fill-opacity: 0.2; }
.file-icon { color: var(--text-muted); }

.btn-icon-sm {
  padding: 0.25rem;
  background: transparent;
  border: none;
  color: var(--text-muted);
  opacity: 0;
  transition: all 0.2s;
}

.file-row:hover .btn-icon-sm { opacity: 1; }
.btn-icon-sm:hover { color: var(--text-main); background: var(--bg-surface-hover); }

.empty-folder {
  padding: 4rem;
  text-align: center;
  color: var(--text-muted);
  font-style: italic;
}
</style>
