/* ============================================================================
   W-Access (Invenzi) — PROVISIONAMENTO EM LOTE de Emuladores
   ----------------------------------------------------------------------------
   Para cada equipamento (loop), faz 3 passos:
     1. INSERT controlador  -> dbo.CfgHWLocalControllers (PK identity)
     2. EXEC  wsp_AddReader -> dbo.CfgHWReaders (leitora ligada ao controlador)
     3. INSERT vínculo nível -> dbo.CfgACAccessLevelsContents (leitora no nível)

   Regras fixas:
     - LanguageID  = 2  (sempre)
     - PartitionID = 0  (Sistema — equipamento)
     - Descrição do controlador = 'emulator_{t}', t = inteiro aleatório [10..210]
       (padrão exigido para o emulador ser reconhecido como válido)
     - Porta de cada equipamento = PortaInicial + (índice - 1)
     - Toda leitora: ReaderHWID = 1 e CardFormat = 11 (UPDATE pós-SP)

   AVISO: valide nomes/tipos de coluna e a assinatura de wsp_AddReader contra a
   versão do W-Access do cliente antes de rodar em produção.
   ============================================================================ */

SET NOCOUNT ON;
SET XACT_ABORT ON;

/* ==================================================================== */
/* 1) PARÂMETROS EDITÁVEIS — mexa apenas aqui                           */
/* ==================================================================== */
DECLARE
    @SiteControllerID     int          = 2,          -- SiteController dos controladores
    @LocalControllerType  smallint     = 22121,      -- código do tipo de device
    @Quantity             int          = 300,         -- quantos equipamentos criar
    @LocalityID           int          = 3,          -- localidade dos controladores
    @BaseCommPortStart    int          = 4002,       -- porta do 1º equipamento (demais = +1)
    @AccessLevelID        int          = 7,          -- nível de acesso onde a leitora entra
    @IPAddress            varchar(100) = N'dev-linux';-- host/IP dos controladores

/* Constantes fixas (não editar sem motivo) */
DECLARE
    @LanguageID  int = 2,   -- sempre 2
    @PartitionID int = 0;   -- 0 = Sistema (equipamento)

/* ==================================================================== */
/* 2) LOOP DE PROVISIONAMENTO                                           */
/* ==================================================================== */
DECLARE
    @i        int = 1,
    @ok       int = 0,
    @fail     int = 0,
    @t        int,
    @port     int,
    @ctrlName varchar(50),
    @ctrlDesc varchar(100),
    @rdrName  varchar(25),
    @rdrDesc  varchar(50),
    @LocalControllerID int,
    @RC       int,
    @intError smallint,
    @strError nvarchar(255),
    @ReaderID int;

WHILE @i <= @Quantity
BEGIN
    BEGIN TRY
        /* t aleatório [10..210] e porta sequencial */
        SET @t    = ABS(CHECKSUM(NEWID())) % 201 + 10;   -- 0..200 +10 => 10..210
        SET @port = @BaseCommPortStart + (@i - 1);

        SET @ctrlDesc = N'emulator_' + CAST(@t AS varchar(10));            -- padrão exigido
        SET @ctrlName = N'CTRL_' + @ctrlDesc;                             -- nome do controlador
        SET @rdrName  = LEFT(N'RDR_' + @ctrlDesc, 25);                    -- varchar(25)
        SET @rdrDesc  = N'Leitora ' + @ctrlDesc;

        /* -------- Passo 1: controlador (identity) -------- */
        INSERT INTO dbo.CfgHWLocalControllers
        ( LocalControllerName, LocalControllerDescription, SiteControllerID,
          PartitionID, LocalControllerType, IPAddress, BaseCommPort,
          LocalControllerEnabled, LocalControllerStatus, LocalityID,
          ForceLocalAuthentication, IsEnrollTerminal )
        VALUES
        ( @ctrlName, @ctrlDesc, @SiteControllerID,
          @PartitionID, @LocalControllerType, @IPAddress, @port,
          1 /*Enabled*/, 0 /*Status*/, @LocalityID,
          1 /*ForceLocalAuth*/, 0 /*IsEnrollTerminal*/ );

        SET @LocalControllerID = CAST(SCOPE_IDENTITY() AS int);

        /* -------- Passo 2: leitora via SP -------- */
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

        /* -------- Passo 2.5: ajustes obrigatórios na leitora -------- */
        /* ReaderHWID=1 e CardFormat=11: a SP não recebe esses campos,
           então corrige por UPDATE pós-criação. */
        UPDATE dbo.CfgHWReaders
        SET ReaderHWID = 1,
            CardFormatID = 11
        WHERE ReaderID = @ReaderID;

        /* -------- Passo 3: vincular leitora ao nível de acesso -------- */
        INSERT INTO dbo.CfgACAccessLevelsContents
        ( AccessLevelID, ReaderType, ReaderID, AccessTimeID,
          ReaderDirection, RequiresEscort, CanEscort )
        VALUES
        ( @AccessLevelID, 0 /*ReaderType*/, @ReaderID, 1 /*AccessTimeID*/,
          0 /*ReaderDirection*/, 0 /*RequiresEscort*/, 0 /*CanEscort*/ );

        SET @ok = @ok + 1;
        PRINT N'[' + CAST(@i AS varchar(10)) + N'/'
            + CAST(@Quantity AS varchar(10)) + N'] OK  '
            + @ctrlDesc + N' | port=' + CAST(@port AS varchar(10))
            + N' | LCID=' + CAST(@LocalControllerID AS varchar(10))
            + N' | ReaderID=' + CAST(@ReaderID AS varchar(10));
    END TRY
    BEGIN CATCH
        SET @fail = @fail + 1;
        PRINT N'[' + CAST(@i AS varchar(10)) + N'/'
            + CAST(@Quantity AS varchar(10)) + N'] ERRO '
            + ISNULL(@ctrlDesc, N'(sem desc)')
            + N' -> ' + ERROR_MESSAGE()
            + ISNULL(N' | strError=' + @strError, N'');
    END CATCH

    SET @i = @i + 1;
END

PRINT N'=== Concluído. Sucesso=' + CAST(@ok AS varchar(10))
    + N' Falhas=' + CAST(@fail AS varchar(10)) + N' ===';

/* ==================================================================== */
/* 3) Conferência — equipamentos criados nesta faixa de portas          */
/* ==================================================================== */
SELECT lc.LocalControllerID, lc.LocalControllerName, lc.LocalControllerDescription,
       lc.BaseCommPort, r.ReaderID, r.ReaderName
FROM dbo.CfgHWLocalControllers lc
LEFT JOIN dbo.CfgHWReaders r ON r.LocalControllerID = lc.LocalControllerID
WHERE lc.LocalControllerDescription LIKE 'emulator[_]%'
  AND lc.BaseCommPort BETWEEN @BaseCommPortStart
                          AND @BaseCommPortStart + @Quantity - 1
ORDER BY lc.BaseCommPort;
