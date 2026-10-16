<script>
  export let title = "Metric";
  export let value = "0";
  export let unit = "";
  export let change = "+0%";
  export let status = "healthy"; // healthy | warning | critical
</script>

<div class="p-5 bg-white dark:bg-gray-800 rounded-2xl shadow-md border border-gray-100 dark:border-gray-700 hover:shadow-lg transition-shadow duration-200">
  <div class="flex items-center justify-between">
    <span class="text-xs font-semibold text-gray-500 uppercase tracking-wider">{title}</span>
    <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium
      {status === 'healthy' ? 'bg-green-100 text-green-800' : ''}
      {status === 'warning' ? 'bg-yellow-100 text-yellow-800' : ''}
      {status === 'critical' ? 'bg-red-100 text-red-800' : ''}">
      {status}
    </span>
  </div>
  <div class="mt-4 flex items-baseline">
    <span class="text-2xl font-bold text-gray-900 dark:text-white">{value}</span>
    {#if unit}
      <span class="ml-1 text-sm font-medium text-gray-500">{unit}</span>
    {/if}
    <span class="ml-auto text-xs font-semibold text-green-600">{change}</span>
  </div>
</div>
