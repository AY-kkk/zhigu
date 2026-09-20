<template>
  <div class="zhigu-page-fill">
    <ResearchChatPane />
    <p v-if="conversation.runError && !conversation.runView" class="err zhigu-reading">{{ conversation.runError }}</p>
  </div>
</template>
<script setup>
import { onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import ResearchChatPane from '../../components/research/ResearchChatPane.vue'
import { useResearchConversation } from '../../stores/researchConversation.js'

const route = useRoute()
const conversation = useResearchConversation()

async function hydrate() {
  await conversation.loadRun(route.params.id)
}

onMounted(hydrate)
watch(() => route.params.id, hydrate)
</script>
<style scoped>
.err { color: var(--zg-danger); padding-top: 12px; }
</style>
