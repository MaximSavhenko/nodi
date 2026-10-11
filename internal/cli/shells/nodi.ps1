# Enable in this session: nodi completion powershell | Out-String | Invoke-Expression
Register-ArgumentCompleter -Native -CommandName nodi -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    try {
        $elements = @($commandAst.CommandElements | Where-Object { $_.Extent.StartOffset -lt $cursorPosition })
        $words = @()
        foreach ($element in $elements | Select-Object -Skip 1) {
            if ($element.Extent.EndOffset -le $cursorPosition) {
                $value = if ($element -is [System.Management.Automation.Language.StringConstantExpressionAst]) { $element.Value } else { $element.Extent.Text }
                $words += $value
            }
        }
        if ($elements.Count -gt 1 -and $elements[-1].Extent.EndOffset -ge $cursorPosition -and $wordToComplete) {
            if ($elements[-1].Extent.EndOffset -eq $cursorPosition -and $words.Count) { $words = @($words | Select-Object -SkipLast 1) }
        }
        $words += $wordToComplete.Trim("'", '"')
        # Windows PowerShell 5.1 drops empty native arguments. JSON keeps the final empty word.
        $request = ConvertTo-Json -InputObject @($words) -Compress
        $program = if ($elements[0] -is [System.Management.Automation.Language.StringConstantExpressionAst]) { $elements[0].Value } else { $elements[0].Extent.Text }
        $savedEncoding=$OutputEncoding
        try {
            $OutputEncoding=[Text.UTF8Encoding]::new($false)
            $candidates = @($request | & $program __complete --json 2>$null)
        } finally { $OutputEncoding=$savedEncoding }
        foreach ($candidate in $candidates) {
            if ($candidate -eq ':files') {
                $prefix=$wordToComplete.Trim("'", '"')
                $parent=if($prefix) {Split-Path -Path $prefix -Parent} else {'.'}
                if (-not $parent) { $parent='.' }
                $leaf=if($prefix) {Split-Path -Path $prefix -Leaf} else {''}
                foreach ($item in Get-ChildItem -LiteralPath $parent -ErrorAction SilentlyContinue) {
                    if ($item.Name.StartsWith($leaf,[StringComparison]::OrdinalIgnoreCase)) {
                        $path=if ($parent -eq '.') { $item.Name } else { Join-Path $parent $item.Name }
                        if ($item.PSIsContainer) { $path += [IO.Path]::DirectorySeparatorChar }
                        $quoted="'"+$path.Replace("'","''")+"'"
                        [System.Management.Automation.CompletionResult]::new($quoted,$item.Name,'ParameterValue',$path)
                    }
                }
            } else {
                [System.Management.Automation.CompletionResult]::new($candidate,$candidate,'ParameterValue',$candidate)
            }
        }
    } catch { }
}
