/* ============================================================================
   W-Access (Invenzi) — INSERT de Leitora via SP wsp_AddReader
   ----------------------------------------------------------------------------
   Diferente do controlador (INSERT manual), a leitora TEM stored procedure de
   provisionamento: dbo.wsp_AddReader. Usar a SP (faz validação + side-effects
   internos que um INSERT cru não faria).

   Assinatura (ordem dos parâmetros):
     @ReaderName                  varchar(25)    IN   nome da leitora
     @ReaderDescription           varchar(50)    IN
     @PartitionID                 int            IN   0=Sistema (equipamento)
     @LanguageID                  int            IN   idioma do nome (ex.: 1)
     @LocalControllerID           int            IN   FK -> CfgHWLocalControllers
     @EscortReaderIDList          varchar(max)   IN   CSV de ReaderIDs escort (NULL = nenhum)
     @MultiFactorAuthReaderIDList varchar(max)   IN   CSV de ReaderIDs MFA (NULL = nenhum)
     @intError                    smallint       OUT  0 = sucesso
     @strError                    nvarchar(255)  OUT  mensagem de erro
     @ReaderID                    int            OUT  ID da leitora criada

   Partição: leitora é EQUIPAMENTO -> PartitionID = 0 (Sistema). Ver vault
   B02 Invenzi / waccess-db-api-reference (seção Partições).

   AVISO: valide a assinatura da SP contra a versão do W-Access do cliente.
   ============================================================================ */

SET NOCOUNT ON;
SET XACT_ABORT ON;

/* ------------------------------------------------------------------ */
/* 1) PARÂMETROS — edite apenas este bloco                            */
/* ------------------------------------------------------------------ */
DECLARE
    @ReaderName                  varchar(25)   = N'Leitora Emulador 01',
    @ReaderDescription           varchar(50)   = N'Leitora facial Dahua (emulador)',
    @PartitionID                 int           = 0,        -- 0=Sistema (equipamento)
    @LanguageID                  int           = 1,        -- idioma do nome
    @LocalControllerID           int           = NULL,     -- <<< FK do controlador (obrigatório)
    @EscortReaderIDList          varchar(max)  = NULL,     -- CSV ou NULL
    @MultiFactorAuthReaderIDList varchar(max)  = NULL;     -- CSV ou NULL

/* ------------------------------------------------------------------ */
/* 2) Resolver @LocalControllerID por nome (opcional)                 */
/*    Comente se já souber o ID e setar acima direto.                 */
/* ------------------------------------------------------------------ */
IF @LocalControllerID IS NULL
BEGIN
    SELECT @LocalControllerID = LocalControllerID
    FROM dbo.CfgHWLocalControllers
    WHERE LocalControllerName = N'Emulador Dahua';   -- ajuste o nome do controlador

    IF @LocalControllerID IS NULL
    BEGIN
        RAISERROR(N'Controlador não encontrado. Defina @LocalControllerID.', 16, 1);
        RETURN;
    END
END

/* ------------------------------------------------------------------ */
/* 3) Chamar a SP                                                     */
/* ------------------------------------------------------------------ */
DECLARE
    @RC       int,
    @intError smallint,
    @strError nvarchar(255),
    @ReaderID int;

BEGIN TRAN;

    EXECUTE @RC = dbo.wsp_AddReader
         @ReaderName
        ,@ReaderDescription
        ,@PartitionID
        ,@LanguageID
        ,@LocalControllerID
        ,@EscortReaderIDList
        ,@MultiFactorAuthReaderIDList
        ,@intError OUTPUT
        ,@strError OUTPUT
        ,@ReaderID OUTPUT;

    IF @intError <> 0 OR @RC <> 0
    BEGIN
        IF @@TRANCOUNT > 0 ROLLBACK TRAN;
        RAISERROR(N'wsp_AddReader falhou. RC=%d intError=%d strError=%s',
                  16, 1, @RC, @intError, @strError);
        RETURN;
    END

COMMIT TRAN;

PRINT N'>> Leitora inserida. ReaderID = ' + CAST(@ReaderID AS varchar(10))
    + N' | LocalControllerID = ' + CAST(@LocalControllerID AS varchar(10));

/* ------------------------------------------------------------------ */
/* 4) Conferência                                                     */
/* ------------------------------------------------------------------ */
SELECT ReaderID, ReaderName, EntryZoneID
FROM dbo.CfgHWReaders
WHERE ReaderID = @ReaderID;
