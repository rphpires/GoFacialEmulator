-- ============================================================
-- Script de Migração de Portas - WXS para GoFacialEmulator
-- ============================================================
--
-- Este script ajusta as portas dos Local Controllers no WXS
-- para o range 4000-4999 esperado pelo Docker.
--
-- IMPORTANTE:
-- 1. Faça BACKUP do banco antes de executar
-- 2. Teste em ambiente de desenvolvimento primeiro
-- 3. Ajuste os valores conforme necessário
--
-- ============================================================

USE W_Access;
GO

-- ============================================================
-- PARTE 1: VERIFICAÇÃO INICIAL
-- ============================================================

PRINT '============================================================';
PRINT 'VERIFICAÇÃO INICIAL';
PRINT '============================================================';
PRINT '';

-- Verificar quantos controladores existem
PRINT '1. Total de Local Controllers:';
SELECT COUNT(*) AS TotalControllers
FROM LocalControllers;
PRINT '';

-- Verificar portas atuais
PRINT '2. Distribuição de portas atuais:';
SELECT
    CASE
        WHEN Port < 4000 THEN 'Abaixo de 4000'
        WHEN Port BETWEEN 4000 AND 4999 THEN 'No range correto (4000-4999)'
        WHEN Port >= 5000 THEN 'Acima de 4999'
    END AS RangeCategoria,
    COUNT(*) AS Quantidade,
    MIN(Port) AS PortaMenor,
    MAX(Port) AS PortaMaior
FROM LocalControllers
GROUP BY
    CASE
        WHEN Port < 4000 THEN 'Abaixo de 4000'
        WHEN Port BETWEEN 4000 AND 4999 THEN 'No range correto (4000-4999)'
        WHEN Port >= 5000 THEN 'Acima de 4999'
    END;
PRINT '';

-- Verificar portas duplicadas
PRINT '3. Verificar portas duplicadas:';
SELECT Port, COUNT(*) AS Duplicados
FROM LocalControllers
GROUP BY Port
HAVING COUNT(*) > 1;

IF @@ROWCOUNT = 0
    PRINT 'Nenhuma porta duplicada encontrada.';
PRINT '';

-- Listar controladores fora do range
PRINT '4. Controladores fora do range 4000-4999:';
SELECT
    LocalControllerID,
    Name,
    IPAddress,
    Port,
    Model,
    Enabled
FROM LocalControllers
WHERE Port < 4000 OR Port > 4999
ORDER BY Port;
PRINT '';

-- ============================================================
-- PARTE 2: BACKUP (OPCIONAL MAS RECOMENDADO)
-- ============================================================

PRINT '============================================================';
PRINT 'CRIANDO TABELA DE BACKUP';
PRINT '============================================================';
PRINT '';

-- Criar backup da tabela antes das mudanças
IF OBJECT_ID('LocalControllers_Backup_PortMigration', 'U') IS NOT NULL
BEGIN
    DROP TABLE LocalControllers_Backup_PortMigration;
    PRINT 'Backup anterior removido.';
END

SELECT *
INTO LocalControllers_Backup_PortMigration
FROM LocalControllers;

PRINT 'Backup criado: LocalControllers_Backup_PortMigration';
PRINT '';

-- ============================================================
-- PARTE 3: MIGRAÇÃO DAS PORTAS
-- ============================================================

PRINT '============================================================';
PRINT 'MIGRANDO PORTAS PARA RANGE 4000-4999';
PRINT '============================================================';
PRINT '';

-- Descomente UMA das estratégias abaixo:

-- ---------------------------------------------------------------
-- ESTRATÉGIA 1: Migração Sequencial (Recomendada)
-- ---------------------------------------------------------------
-- Atribui portas sequenciais começando de 4000
-- Ideal quando você tem controle total sobre as portas

/*
PRINT 'Usando ESTRATÉGIA 1: Migração Sequencial';
PRINT '';

DECLARE @NextPort INT = 4000;

UPDATE LocalControllers
SET
    Port = @NextPort + (ROW_NUMBER() OVER (ORDER BY LocalControllerID) - 1)
WHERE Port < 4000 OR Port > 4999;

PRINT 'Portas migradas sequencialmente a partir de 4000.';
*/

-- ---------------------------------------------------------------
-- ESTRATÉGIA 2: Preservar Offset Relativo
-- ---------------------------------------------------------------
-- Mantém a diferença relativa entre as portas
-- Útil quando a ordem das portas tem significado

/*
PRINT 'Usando ESTRATÉGIA 2: Preservar Offset Relativo';
PRINT '';

DECLARE @MinPort INT = (SELECT MIN(Port) FROM LocalControllers);
DECLARE @BasePort INT = 4000;

UPDATE LocalControllers
SET Port = @BasePort + (Port - @MinPort)
WHERE Port < 4000 OR Port > 4999;

PRINT 'Portas migradas preservando offset relativo.';
*/

-- ---------------------------------------------------------------
-- ESTRATÉGIA 3: Mapeamento Manual por LocalControllerID
-- ---------------------------------------------------------------
-- Use quando você quer portas específicas para controladores específicos

/*
PRINT 'Usando ESTRATÉGIA 3: Mapeamento Manual';
PRINT '';

UPDATE LocalControllers
SET Port =
    CASE LocalControllerID
        WHEN 1 THEN 4001
        WHEN 2 THEN 4002
        WHEN 3 THEN 4003
        -- Adicione mais mapeamentos conforme necessário
        ELSE 4000 + LocalControllerID
    END
WHERE Port < 4000 OR Port > 4999;

PRINT 'Portas migradas usando mapeamento manual.';
*/

-- ---------------------------------------------------------------
-- ESTRATÉGIA 4: Offset Baseado em LocalControllerID (PADRÃO)
-- ---------------------------------------------------------------
-- Usa LocalControllerID como base (mais previsível)
-- Esta é a estratégia ativa por padrão

PRINT 'Usando ESTRATÉGIA 4: Offset baseado em LocalControllerID (PADRÃO)';
PRINT '';

-- Verificar se teremos conflitos (IDs muito altos)
IF EXISTS (SELECT 1 FROM LocalControllers WHERE 4000 + LocalControllerID > 4999)
BEGIN
    PRINT 'AVISO: Alguns LocalControllerIDs são muito altos!';
    PRINT 'IDs que causariam problema:';
    SELECT LocalControllerID, 4000 + LocalControllerID AS PortaCalculada
    FROM LocalControllers
    WHERE 4000 + LocalControllerID > 4999;
    PRINT '';
    PRINT 'Use ESTRATÉGIA 1 (sequencial) neste caso.';
    PRINT '';
END
ELSE
BEGIN
    -- Executar migração
    UPDATE LocalControllers
    SET Port = 4000 + LocalControllerID
    WHERE Port < 4000 OR Port > 4999;

    PRINT 'Portas migradas usando LocalControllerID como offset.';
    PRINT '';
END

-- ============================================================
-- PARTE 4: VERIFICAÇÃO PÓS-MIGRAÇÃO
-- ============================================================

PRINT '============================================================';
PRINT 'VERIFICAÇÃO PÓS-MIGRAÇÃO';
PRINT '============================================================';
PRINT '';

-- Verificar se todas as portas estão no range correto
PRINT '1. Portas fora do range após migração:';
SELECT
    LocalControllerID,
    Name,
    Port
FROM LocalControllers
WHERE Port < 4000 OR Port > 4999;

IF @@ROWCOUNT = 0
    PRINT 'Todas as portas estão no range correto (4000-4999)!';
PRINT '';

-- Verificar duplicatas após migração
PRINT '2. Verificar duplicatas após migração:';
SELECT Port, COUNT(*) AS Duplicados
FROM LocalControllers
GROUP BY Port
HAVING COUNT(*) > 1;

IF @@ROWCOUNT = 0
    PRINT 'Nenhuma porta duplicada!';
PRINT '';

-- Mostrar distribuição final
PRINT '3. Distribuição final de portas:';
SELECT
    MIN(Port) AS PortaMenor,
    MAX(Port) AS PortaMaior,
    COUNT(*) AS TotalControladores,
    COUNT(DISTINCT Port) AS PortasUnicas
FROM LocalControllers;
PRINT '';

-- Listar todos os controladores com suas novas portas
PRINT '4. Lista completa de controladores e portas:';
SELECT
    LocalControllerID,
    Name,
    IPAddress,
    Port,
    Model,
    Enabled
FROM LocalControllers
ORDER BY Port;
PRINT '';

-- ============================================================
-- PARTE 5: ROLLBACK (SE NECESSÁRIO)
-- ============================================================

/*
-- Descomente este bloco para fazer rollback

PRINT '============================================================';
PRINT 'FAZENDO ROLLBACK DA MIGRAÇÃO';
PRINT '============================================================';
PRINT '';

UPDATE lc
SET lc.Port = bkp.Port
FROM LocalControllers lc
INNER JOIN LocalControllers_Backup_PortMigration bkp
    ON lc.LocalControllerID = bkp.LocalControllerID;

PRINT 'Rollback concluído! Portas restauradas do backup.';
PRINT '';
*/

-- ============================================================
PRINT '============================================================';
PRINT 'MIGRAÇÃO CONCLUÍDA!';
PRINT '============================================================';
PRINT '';
PRINT 'Próximos passos:';
PRINT '1. Verifique os resultados acima';
PRINT '2. Reinicie o GoFacialEmulator';
PRINT '3. Atualize dispositivos via API: POST /api/devices/refresh';
PRINT '4. Teste os emuladores: curl http://localhost:<porta>/ISAPI/System/deviceInfo';
PRINT '';
PRINT 'Se algo deu errado, descomente o bloco ROLLBACK e execute novamente.';
PRINT '============================================================';
