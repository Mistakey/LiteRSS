import { onBeforeUnmount, onMounted, ref } from 'vue';

/** 当前 Unix 秒，每 30 秒更新一次，给相对时间文案用。 */
export function useNow() {
  const now = ref(Math.floor(Date.now() / 1000));
  let timer: ReturnType<typeof setInterval> | undefined;
  onMounted(() => {
    timer = setInterval(() => (now.value = Math.floor(Date.now() / 1000)), 30_000);
  });
  onBeforeUnmount(() => clearInterval(timer));
  return now;
}
