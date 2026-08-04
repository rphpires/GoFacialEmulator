/* ============================================================================
   W-Access (Invenzi) — REMEDIAÇÃO: cria leitoras faltantes
   ----------------------------------------------------------------------------
   Contexto: bulk-insert-emulators.sql falhou parcialmente. Vários
   CfgHWLocalControllers (emulator_*) ficaram SEM leitora associada.

   Este script, para cada controlador emulator SEM leitora:
     1. EXEC  wsp_AddReader  -> cria leitora ligada ao controlador
     2. UPDATE CfgHWReaders   -> ReaderHWID=1, CardFormatID=11 (igual ao bulk)
     3. INSERT CfgACAccessLevelsContents -> vincula leitora ao nível de acesso

   Idempotente: só processa controladores que NÃO têm leitora; e só insere
   no nível de acesso se o vínculo ainda não existir. Rodar de novo é seguro.

   AVISO: valide assinatura de wsp_AddReader contra a versão do cliente.
   ============================================================================ */

SET NOCOUNT ON;
SET XACT_ABORT ON;

/* ==================================================================== */
/* 1) PARÂMETROS EDITÁVEIS                                              */
/* ==================================================================== */
DECLARE
    @AccessLevelID  int = 7;   -- nível de acesso onde a leitora entra (mesmo do bulk)

/* Constantes fixas (mesmas do bulk-insert) */
DECLARE
    @LanguageID  int = 2,   -- sempre 2
    @PartitionID int = 0;   -- 0 = Sistema (equipamento)

/* ==================================================================== */
/* 2) DIAGNÓSTICO — controladores emulator SEM leitora                  */
/* ==================================================================== */
DECLARE @orphans TABLE
(
    rn                 int IDENTITY(1,1) PRIMARY KEY,
    LocalControllerID  int,
    LocalControllerDescription varchar(100),
    BaseCommPort       int
);

INSERT INTO @orphans (LocalControllerID, LocalControllerDescription, BaseCommPort)
SELECT lc.LocalControllerID, lc.LocalControllerDescription, lc.BaseCommPort
FROM dbo.CfgHWLocalControllers lc
WHERE lc.LocalControllerDescription LIKE 'emulator[_]%'
  AND NOT EXISTS (
        SELECT 1 FROM dbo.CfgHWReaders r
        WHERE r.LocalControllerID = lc.LocalControllerID
  )
ORDER BY lc.BaseCommPort;

DECLARE @total int = (SELECT COUNT(*) FROM @orphans);
PRINT N'Controladores emulator SEM leitora encontrados: ' + CAST(@total AS varchar(10));

/* ==================================================================== */
/* 3) LOOP DE REMEDIAÇÃO                                                */
/* ==================================================================== */
DECLARE
    @i        int = 1,
    @ok       int = 0,
    @fail     int = 0,
    @LocalControllerID int,
    @ctrlDesc varchar(100),
    @ctrlName varchar(50),
    @rdrName  varchar(25),
    @rdrDesc  varchar(50),
    @suffix   varchar(12),
    @RC       int,
    @intError smallint,
    @strError nvarchar(255),
    @ReaderID int;

WHILE @i <= @total
BEGIN
    SELECT @LocalControllerID = LocalControllerID,
           @ctrlDesc          = LocalControllerDescription
    FROM @orphans WHERE rn = @i;

    BEGIN TRY
        /* Sufixo único = _{LocalControllerID} (identity, nunca colide). */
        SET @suffix  = N'_' + CAST(@LocalControllerID AS varchar(10));

        /* Nomes truncados ao limite da coluna PRESERVANDO o sufixo no fim,
           senão o corte poderia gerar duplicata de novo. */
        SET @ctrlName = LEFT(N'CTRL_' + @ctrlDesc, 50 - LEN(@suffix)) + @suffix;  -- varchar(50)
        SET @rdrName  = LEFT(N'RDR_'  + @ctrlDesc, 25 - LEN(@suffix)) + @suffix;  -- varchar(25)
        SET @rdrDesc  = N'Leitora ' + @ctrlDesc + @suffix;

        /* -------- Passo 0: renomear controlador p/ nome único -------- */
        UPDATE dbo.CfgHWLocalControllers
        SET LocalControllerName = @ctrlName
        WHERE LocalControllerID = @LocalControllerID;

        /* -------- Passo 1: leitora via SP -------- */
        SET @intError = 0; SET @strError = NULL; SET @ReaderID = NULL;

        EXEC @RC = dbo.wsp_AddReader
             @rdrName
            ,@rdrDesc
            ,@PartitionID
            ,@LanguageID
            ,@LocalControllerID
            ,NULL              -- @EscortReaderIDList
            ,NULL              -- @MultiFactorAuthReaderIDList
            ,@intError OUTPUT
            ,@strError OUTPUT
            ,@ReaderID OUTPUT;

        IF @RC <> 0 OR @intError <> 0 OR @ReaderID IS NULL
            THROW 50001, N'wsp_AddReader falhou', 1;

        /* -------- Passo 2: ajustes obrigatórios na leitora -------- */
        UPDATE dbo.CfgHWReaders
        SET ReaderHWID = 1,
            CardFormatID = 11
        WHERE ReaderID = @ReaderID;

        /* -------- Passo 3: vincular ao nível (se ainda não existir) -------- */
        IF NOT EXISTS (
            SELECT 1 FROM dbo.CfgACAccessLevelsContents
            WHERE AccessLevelID = @AccessLevelID
              AND ReaderType = 0
              AND ReaderID = @ReaderID
        )
        BEGIN
            INSERT INTO dbo.CfgACAccessLevelsContents
            ( AccessLevelID, ReaderType, ReaderID, AccessTimeID,
              ReaderDirection, RequiresEscort, CanEscort )
            VALUES
            ( @AccessLevelID, 0 /*ReaderType*/, @ReaderID, 1 /*AccessTimeID*/,
              0 /*ReaderDirection*/, 0 /*RequiresEscort*/, 0 /*CanEscort*/ );
        END

        SET @ok = @ok + 1;
        PRINT N'[' + CAST(@i AS varchar(10)) + N'/' + CAST(@total AS varchar(10))
            + N'] OK  ' + @ctrlDesc
            + N' | LCID=' + CAST(@LocalControllerID AS varchar(10))
            + N' | ReaderID=' + CAST(@ReaderID AS varchar(10));
    END TRY
    BEGIN CATCH
        SET @fail = @fail + 1;
        PRINT N'[' + CAST(@i AS varchar(10)) + N'/' + CAST(@total AS varchar(10))
            + N'] ERRO ' + ISNULL(@ctrlDesc, N'(sem desc)')
            + N' (LCID=' + CAST(@LocalControllerID AS varchar(10)) + N')'
            + N' -> ' + ERROR_MESSAGE()
            + ISNULL(N' | strError=' + @strError, N'');
    END CATCH

    SET @i = @i + 1;
END

PRINT N'=== Concluído. Sucesso=' + CAST(@ok AS varchar(10))
    + N' Falhas=' + CAST(@fail AS varchar(10)) + N' ===';

/* ==================================================================== */
/* 4) CONFERÊNCIA — deve sobrar ZERO controlador emulator sem leitora   */
/* ==================================================================== */
SELECT lc.LocalControllerID, lc.LocalControllerName, lc.LocalControllerDescription,
       lc.BaseCommPort, r.ReaderID, r.ReaderName,
       alc.AccessLevelID
FROM dbo.CfgHWLocalControllers lc
LEFT JOIN dbo.CfgHWReaders r ON r.LocalControllerID = lc.LocalControllerID
LEFT JOIN dbo.CfgACAccessLevelsContents alc
       ON alc.ReaderID = r.ReaderID AND alc.AccessLevelID = @AccessLevelID
WHERE lc.LocalControllerDescription LIKE 'emulator[_]%'
ORDER BY (CASE WHEN r.ReaderID IS NULL THEN 0 ELSE 1 END), lc.BaseCommPort;
