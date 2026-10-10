import { createApp } from 'vue'
import App from './App.vue'

// 本地打包的 IBM Plex（latin 子集）：面板离线可用，不依赖外部字体服务
import '@fontsource/ibm-plex-sans/latin-400.css'
import '@fontsource/ibm-plex-sans/latin-500.css'
import '@fontsource/ibm-plex-sans/latin-600.css'
import '@fontsource/ibm-plex-mono/latin-400.css'
import '@fontsource/ibm-plex-mono/latin-500.css'
import '@fontsource/ibm-plex-mono/latin-600.css'

import './style.css'

createApp(App).mount('#app')
