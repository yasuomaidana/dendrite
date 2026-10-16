<script>
  export let label = "Action";
  export let variant = "primary"; // primary | secondary | danger | ghost
  export let size = "md"; // sm | md | lg
  export let disabled = false;
  export let loading = false;
</script>

<button
  type="button"
  disabled={disabled || loading}
  class="inline-flex items-center justify-center font-medium transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-offset-2
    {variant === 'primary' ? 'bg-primary-600 text-white hover:bg-primary-700 active:bg-primary-800 focus:ring-primary-500 shadow-sm' : ''}
    {variant === 'secondary' ? 'bg-gray-100 text-gray-900 hover:bg-gray-200 active:bg-gray-300 focus:ring-gray-400 border border-gray-300' : ''}
    {variant === 'danger' ? 'bg-red-600 text-white hover:bg-red-700 active:bg-red-800 focus:ring-red-500 shadow-sm' : ''}
    {size === 'sm' ? 'px-3 py-1.5 text-xs rounded-lg gap-1.5' : ''}
    {size === 'md' ? 'px-4 py-2 text-sm rounded-xl gap-2' : ''}
    {size === 'lg' ? 'px-6 py-3 text-base rounded-2xl gap-3' : ''}
    {disabled ? 'opacity-50 cursor-not-allowed shadow-none' : 'cursor-pointer'}
    dark:focus:ring-offset-gray-900"
  on:click
>
  {#if loading}
    <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-current" fill="none" viewBox="0 0 24 24">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
    </svg>
  {/if}
  <span>{label}</span>
</button>
